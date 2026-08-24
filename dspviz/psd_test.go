package dspviz

import (
	"math"
	"math/rand/v2"
	"testing"
)

// TestPSDFullScale pins the scaling chain the whole package hangs off: a
// full-scale coherent sine reads exactly 0 dBFS in exactly one bin, and all
// three curves agree there because a steady tone has nothing to vary.
func TestPSDFullScale(t *testing.T) {
	const (
		size = 1024
		rate = 8000.0
		bin  = 64
	)
	x := make([]float64, 8*size)
	for i := range x {
		x[i] = math.Cos(2 * math.Pi * float64((bin*i)%size) / size)
	}
	p, err := PSDOf(x, rate, STFTOptions{Size: size, Window: rectangular})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		v    []float64
	}{{"average", p.AvgDB}, {"peak", p.MaxDB}, {"minimum", p.MinDB}} {
		if math.Abs(c.v[bin]) > 1e-9 {
			t.Errorf("%s at bin %d is %v dBFS, want 0", c.name, bin, c.v[bin])
		}
	}
	loud := 0
	for _, v := range p.AvgDB {
		if v > -100 {
			loud++
		}
	}
	if loud != 1 {
		t.Errorf("%d bins are above -100 dBFS, want 1", loud)
	}
}

// TestPSDComplexFullScale is the factor-of-two test again, on the averaged
// spectrum: keeping the real path's doubling here would report every I/Q
// capture 6.0206 dB hot and nothing on the plot would look wrong.
func TestPSDComplexFullScale(t *testing.T) {
	const (
		size = 512
		rate = 8000.0
		bin  = 100
	)
	x := make([]complex128, 8*size)
	for i := range x {
		th := 2 * math.Pi * float64((bin*i)%size) / size
		x[i] = complex(math.Cos(th), math.Sin(th))
	}
	p, err := PSDOfComplex(x, rate, STFTOptions{Size: size, Window: rectangular})
	if err != nil {
		t.Fatal(err)
	}
	if !p.TwoSided() {
		t.Fatal("a complex spectrum should be two-sided")
	}
	got := p.AvgDB[bin+size/2]
	if math.Abs(got) > 1e-9 {
		t.Errorf("a full-scale exponential reads %v dBFS, want 0 (6.0206 dB means the factor of two is still there)", got)
	}
}

// TestPSDParseval is what catches a wrong window normalization, which nothing on
// a plot would show: every level would be wrong by the same factor, so the
// picture would look entirely reasonable.
//
// The equivalent noise bandwidth is what converts a sum over bins back into
// total power, and it has to work for a tone -- whose energy the window spreads
// over its main lobe by exactly that factor -- as well as for noise.
func TestPSDParseval(t *testing.T) {
	const (
		size = 1024
		rate = 8000.0
	)
	tests := []struct {
		name string
		want float64
		gen  func(i int) float64
	}{
		{"tone on a bin", 0.5 * 0.5 * 0.5, func(i int) float64 {
			return 0.5 * math.Cos(2*math.Pi*float64((37*i)%size)/size)
		}},
		{"tone off a bin", 0.5 * 0.5 * 0.5, func(i int) float64 {
			return 0.5 * math.Cos(2*math.Pi*37.4*float64(i)/size)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := make([]float64, 64*size)
			for i := range x {
				x[i] = tt.gen(i)
			}
			p, err := PSDOf(x, rate, STFTOptions{Size: size})
			if err != nil {
				t.Fatal(err)
			}
			got := p.MeanSquare()
			if math.Abs(got-tt.want)/tt.want > 0.01 {
				t.Errorf("mean square %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("white noise", func(t *testing.T) {
		r := rand.New(rand.NewPCG(3, 5))
		x := make([]float64, 256*size)
		var want float64
		for i := range x {
			x[i] = 0.1 * r.NormFloat64()
			want += x[i] * x[i]
		}
		want /= float64(len(x))
		p, err := PSDOf(x, rate, STFTOptions{Size: size})
		if err != nil {
			t.Fatal(err)
		}
		got := p.MeanSquare()
		if math.Abs(got-want)/want > 0.02 {
			t.Errorf("mean square %v, want %v", got, want)
		}
	})

	t.Run("complex noise", func(t *testing.T) {
		r := rand.New(rand.NewPCG(11, 13))
		x := make([]complex128, 256*size)
		var want float64
		for i := range x {
			re, im := 0.1*r.NormFloat64(), 0.1*r.NormFloat64()
			x[i] = complex(re, im)
			want += re*re + im*im
		}
		want /= float64(len(x))
		p, err := PSDOfComplex(x, rate, STFTOptions{Size: size})
		if err != nil {
			t.Fatal(err)
		}
		got := p.MeanSquare()
		if math.Abs(got-want)/want > 0.02 {
			t.Errorf("mean square %v, want %v", got, want)
		}
	})
}

// TestPSDNeverAveragesDecibels is the mistake the accumulation exists to avoid.
//
// Averaging the spectrogram's rows is a geometric mean of power, which for
// Rayleigh-magnitude noise sits about 2.5 dB below the true mean while leaving a
// coherent tone alone -- so the plot silently overstates the signal-to-noise
// ratio by an amount that depends on how noise-like the bin is.
func TestPSDNeverAveragesDecibels(t *testing.T) {
	const (
		size = 512
		rate = 8000.0
	)
	r := rand.New(rand.NewPCG(17, 19))
	x := make([]float64, 512*size)
	for i := range x {
		x[i] = 0.05 * r.NormFloat64()
	}
	b, err := NewSpectrogramBuilder(rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Write(x); err != nil {
		t.Fatal(err)
	}
	sg, err := b.Close()
	if err != nil {
		t.Fatal(err)
	}
	p := b.PSD()

	// The same bins, averaged the wrong way.
	const bin = 100
	var db float64
	for _, row := range sg.Frames {
		db += row[bin]
	}
	db /= float64(len(sg.Frames))

	gap := p.AvgDB[bin] - db
	if gap < 1.5 || gap > 4 {
		t.Errorf("the decibel mean is %.2f dB below the power mean; expected the ~2.5 dB Rayleigh gap", gap)
	}
}

func TestOccupiedBandwidth(t *testing.T) {
	const (
		size = 1024
		rate = 8000.0
	)
	// Two tones 500 Hz apart with nothing between them: 99% of the power is the
	// span that holds both.
	x := make([]float64, 32*size)
	for i := range x {
		x[i] = 0.5*math.Cos(2*math.Pi*float64((128*i)%size)/size) +
			0.5*math.Cos(2*math.Pi*float64((192*i)%size)/size)
	}
	p, err := PSDOf(x, rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := OccupiedBandwidth(p, 0.99)
	wantLo, wantHi := 128*rate/size, 192*rate/size
	if math.Abs(lo-wantLo) > 40 || math.Abs(hi-wantHi) > 40 {
		t.Errorf("occupied band %v..%v Hz, want about %v..%v", lo, hi, wantLo, wantHi)
	}

	// And the -3 dB width of the strongest is one window main lobe, not the
	// whole band.
	if blo, bhi := BandwidthAtDB(p, 3); bhi-blo > 200 {
		t.Errorf("-3 dB width %v Hz, want a main lobe", bhi-blo)
	}
}

func TestPSDNoiseFloorAndFlatness(t *testing.T) {
	const (
		size = 512
		rate = 8000.0
	)
	r := rand.New(rand.NewPCG(23, 29))
	noise := make([]float64, 64*size)
	tone := make([]float64, 64*size)
	for i := range noise {
		noise[i] = 0.01 * r.NormFloat64()
		tone[i] = math.Cos(2 * math.Pi * float64((77*i)%size) / size)
	}
	np, err := PSDOf(noise, rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	tp, err := PSDOf(tone, rate, STFTOptions{Size: size})
	if err != nil {
		t.Fatal(err)
	}
	if f := np.Flatness(); f < 0.5 {
		t.Errorf("white noise has flatness %v, want near 1", f)
	}
	if f := tp.Flatness(); f > 0.01 {
		t.Errorf("a pure tone has flatness %v, want near 0", f)
	}
	// A percentile below the middle is not pulled up by the tone: the floor of a
	// spectrum holding a full-scale tone still reads as the floor.
	if got := tp.NoiseFloorDB(20); got > -100 {
		t.Errorf("the floor under a full-scale tone reads %v dBFS, want it far below the tone", got)
	}
}

// TestPeakBinSkipsDCNotTheEnds pins which bins PeakBin excludes.
//
// It used to skip by index from the two ends of the array. For a one-sided
// spectrum that is the same thing as skipping DC, so it looked right -- but
// FFTShift puts a two-sided spectrum's zero in the middle and the band edges at
// the ends, so the DC spur it exists to exclude was kept and the two edges were
// thrown away. SpectralStatsOf asks for exactly that.
func TestPeakBinSkipsDCNotTheEnds(t *testing.T) {
	// A two-sided layout: -4..+3 Hz, so zero sits at index 4.
	p := &PSD{
		Freq:  []float64{-4, -3, -2, -1, 0, 1, 2, 3},
		AvgDB: []float64{-50, -50, -50, -50, 0, -50, -50, -20},
	}
	if !p.TwoSided() {
		t.Fatal("the fixture is not two-sided")
	}

	// With no skip the DC spur is the loudest thing there.
	if bin, _ := p.PeakBin(0); bin != 4 {
		t.Errorf("PeakBin(0) = %d, want the DC bin 4", bin)
	}
	// With a skip it has to be excluded, and the real peak found instead.
	bin, db := p.PeakBin(2)
	if bin != 7 || db != -20 {
		t.Errorf("PeakBin(2) = bin %d at %v dB, want bin 7 at -20", bin, db)
	}

	// A band edge is a candidate, not something to skip: put the peak at the
	// very end and it still has to be found.
	p.AvgDB = []float64{-50, -50, -50, -50, 0, -50, -50, -10}
	if bin, _ := p.PeakBin(2); bin != 7 {
		t.Errorf("PeakBin(2) = %d, want the last bin", bin)
	}
	p.AvgDB = []float64{-10, -50, -50, -50, 0, -50, -50, -50}
	if bin, _ := p.PeakBin(2); bin != 0 {
		t.Errorf("PeakBin(2) = %d, want the first bin", bin)
	}
}
