package firpm

import (
	"errors"
	"math"
	"slices"
	"testing"
)

// closeEnough compares with a relative tolerance and an absolute floor, since
// individual impulse response coefficients pass through zero.
func closeEnough(got, want, tol float64) bool {
	d := math.Abs(got - want)
	if d <= tol {
		return true
	}
	return d <= tol*math.Abs(want)
}

// checkTrajectory compares the exchange's interior state against the reference
// dump. Integers are exact on both sides whatever the arithmetic precision, so
// a mismatch here is a transliteration bug and not a rounding difference.
func checkTrajectory(t *testing.T, want, got *goldenCase, exact bool, devTol float64) {
	t.Helper()

	if len(got.iters) != len(want.iters) {
		t.Errorf("ran %d iterations, reference ran %d", len(got.iters), len(want.iters))
	}
	n := min(len(got.iters), len(want.iters))
	for i := range n {
		w, g := want.iters[i], got.iters[i]
		if g.niter != w.niter || g.luck != w.luck || g.jchnge != w.jchnge {
			t.Errorf("iteration %d: niter/luck/jchnge = %d/%d/%d, want %d/%d/%d",
				i+1, g.niter, g.luck, g.jchnge, w.niter, w.luck, w.jchnge)
		}
		if !slices.Equal(g.iext, w.iext) {
			t.Errorf("iteration %d: iext = %v\n                want %v", i+1, g.iext, w.iext)
			// Every later iteration follows from this one, so stop.
			return
		}
		if !closeEnough(g.dev, w.dev, devTol) {
			t.Errorf("iteration %d: deviation = %.17g, want %.17g", i+1, g.dev, w.dev)
		}
	}
	if !exact {
		return
	}
	if got.niter != want.niter || got.kkk != want.kkk || got.luck != want.luck || got.jchnge != want.jchnge {
		t.Errorf("final niter/kkk/luck/jchnge = %d/%d/%d/%d, want %d/%d/%d/%d",
			got.niter, got.kkk, got.luck, got.jchnge,
			want.niter, want.kkk, want.luck, want.jchnge)
	}
}

// checkImpulse compares the first half of the impulse response against the
// reference and then checks that the Go side mirrored it correctly, which the
// reference does not print. tol is relative to the largest coefficient, since
// individual coefficients pass through zero.
func checkImpulse(t *testing.T, want *goldenCase, h []float64, tol float64) {
	t.Helper()

	if len(h) != want.numtaps {
		t.Fatalf("got %d taps, want %d", len(h), want.numtaps)
	}
	scale := want.peak() * tol
	for j, w := range want.h {
		if math.Abs(h[j]-w) > scale {
			t.Errorf("h[%d] = %.17g, want %.17g (delta %.3g, tolerance %.3g)",
				j+1, h[j], w, h[j]-w, scale)
		}
	}
	sign := 1.0
	if want.neg == 1 {
		sign = -1.0
	}
	for j := range want.numtaps {
		k := want.numtaps - 1 - j
		if h[k] != sign*h[j] {
			t.Errorf("h[%d] = %.17g, not %v times h[%d] = %.17g", k+1, h[k], sign, j+1, h[j])
		}
	}
}

// TestDesignGolden checks the port against the same Fortran built with
// -fdefault-real-8, which computes in float64 throughout as this package does.
// The interior state has to match exactly; only the arithmetic may drift, by
// the last bit or two. Over the corpus the largest disagreement is 1.1e-13 on
// the deviation and 7.3e-15 on a coefficient of a converged design, from
// math.Cos against the platform's, summation order, and the one deliberate
// improvement: math.Acos where the reference uses ATAN2(SQRT(1-XT*XT),XT).
// noconverge reaches 6.4e-14, what 25 iterations of a design that never
// settles do to the last digits.
func TestDesignGolden(t *testing.T) {
	for _, want := range loadGoldens(t, "r8") {
		t.Run(want.name, func(t *testing.T) {
			h, dev, got, err := want.run(t)
			if !errors.Is(err, want.wantErr()) {
				t.Fatalf("error = %v, want %v", err, want.wantErr())
			}
			checkTrajectory(t, want, got, true, 1e-11)
			if !closeEnough(dev, want.dev, 1e-11) {
				t.Errorf("deviation = %.17g, want %.17g", dev, want.dev)
			}
			checkImpulse(t, want, h, 1e-12)
		})
	}
}

// TestDesignFortranSingle checks the port against the historical program,
// whose REAL variables are 4 bytes: grid, weights, ALPHA, impulse response and
// the ERR/COMP comparison that drives the search are all single precision.
// Agreement is therefore only to that precision. The worst case over the
// corpus is 5.3e-5 on the deviation and 2.7e-5 on a coefficient, both from
// diff31, against a float32 epsilon of 1.2e-7 -- a few thousand
// single-precision operations accumulated, not a disagreement about the
// answer.
//
// Whether the exchange converges at all can also turn on precision: multiband
// runs out of improvement and takes the reference's OUCH exit at single
// precision, while at double precision it converges in 19 iterations. Cases
// where the two disagree about that are skipped.
//
// Two cases do not describe the same problem at all: DELF is single precision
// and the grid is built by repeated addition of it, so the single- and
// double-precision builds disagree about NGRID -- short5 lands on 45 grid
// points against 44, multiband on 192 against 193. Those are skipped rather
// than compared loosely, since a loose bound there would hide a real defect.
func TestDesignFortranSingle(t *testing.T) {
	for _, want := range loadGoldens(t, "r4") {
		t.Run(want.name, func(t *testing.T) {
			h, dev, got, err := want.run(t)
			if got.ngrid != want.ngrid {
				t.Skipf("single precision DELF puts %d points on the grid, double precision %d; "+
					"the two builds are solving different problems", want.ngrid, got.ngrid)
			}
			if want.wantErr() != nil || err != nil {
				t.Skipf("convergence differs by precision: the reference exited with %v, this port with %v",
					want.wantErr(), err)
			}
			if !closeEnough(dev, want.dev, 2e-4) {
				t.Errorf("deviation = %.17g, want %.17g", dev, want.dev)
			}
			checkImpulse(t, want, h, 2e-4)
		})
	}
}

// TestDesignNonConvergence covers the two exits the reference takes by warning
// and carrying on. Both must still produce the filter the reference produces.
//
// Only the iteration limit is in the corpus. The other exit fires when the
// deviation stops improving, which is a rounding effect: at single precision
// the reference reaches it (multiband in the r4 corpus does, and so does a
// length 128 five-band design), but at the float64 this package and the r8
// build use, it does not arise in any design tried here. Both exits leave the
// exchange by the same path, so the corpus case covers the behavior.
func TestDesignNonConvergence(t *testing.T) {
	var want *goldenCase
	for _, g := range loadGoldens(t, "r8") {
		if g.fail == 1 {
			want = g
			break
		}
	}
	if want == nil {
		t.Fatal("no golden case reaches the iteration limit")
	}

	h, dev, err := Design(want.spec())
	if !errors.Is(err, ErrMaxIterations) {
		t.Fatalf("error = %v, want ErrMaxIterations", err)
	}
	if h == nil {
		t.Fatal("no coefficients returned; the reference returns the design it reached")
	}
	if !closeEnough(dev, want.dev, 1e-11) {
		t.Errorf("deviation = %.17g, want %.17g", dev, want.dev)
	}
	checkImpulse(t, want, h, 1e-12)
}

// TestDesignEquiripple checks the defining property of the design directly,
// without reference to the Fortran: the weighted error must reach the reported
// deviation across each band and never exceed it.
func TestDesignEquiripple(t *testing.T) {
	for _, want := range loadGoldens(t, "r8") {
		if want.fail != 0 {
			// The error curve of a design that ran out of iterations is
			// not equiripple, which is why it ran out.
			continue
		}
		if want.filterType != BandPass {
			// eff and wate make the error curve of the other two types a
			// function of frequency; the plain magnitude check below only
			// describes the bandpass case.
			continue
		}
		t.Run(want.name, func(t *testing.T) {
			h, dev, _, err := want.run(t)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// The design is only optimal on the grid; between grid points
			// the continuous response is free to overshoot, and it does so
			// by roughly the square of the spacing. Measured over this
			// corpus that is 2.7% at the reference density of 16, 0.1% at
			// 32 and 15% at 4, so allow a little over the first of those.
			over := 1.0 + 10.0/float64(want.gridDensity*want.gridDensity)
			for b := range len(want.bands) / 2 {
				lo, hi := want.bands[2*b], want.bands[2*b+1]
				ripple := dev / want.weight[b]
				var peak float64
				const steps = 512
				for i := range steps + 1 {
					f := lo + (hi-lo)*float64(i)/steps
					e := math.Abs(response(h, f) - want.response[b])
					peak = math.Max(peak, e)
					if e > ripple*over+1e-9 {
						t.Errorf("band %d: error %.6g at f=%.6g exceeds ripple %.6g", b+1, e, f, ripple)
						break
					}
				}
				// An equiripple design touches the bound in every band.
				if peak < ripple*0.98 {
					t.Errorf("band %d: peak error %.6g never reaches ripple %.6g", b+1, peak, ripple)
				}
			}
		})
	}
}

// response evaluates the real frequency response of h at normalized frequency
// f, in cycles per sample.
func response(h []float64, f float64) float64 {
	var re, im float64
	for n, c := range h {
		s, co := math.Sincos(pi2 * f * float64(n))
		re += c * co
		im -= c * s
	}
	return math.Hypot(re, im)
}

func TestDesignValidation(t *testing.T) {
	ok := []Band{{0.0, 0.2, 1.0, 1.0}, {0.25, 0.5, 0.0, 1.0}}

	tests := []struct {
		name string
		spec Spec
	}{
		{"too few taps", Spec{NumTaps: 2, Bands: ok}},
		{"negative taps", Spec{NumTaps: -1, Bands: ok}},
		{"unknown type", Spec{NumTaps: 21, Bands: ok, Type: FilterType(3)}},
		{"negative type", Spec{NumTaps: 21, Bands: ok, Type: FilterType(-1)}},
		{"no bands", Spec{NumTaps: 21}},
		{"edge above nyquist", Spec{NumTaps: 21, Bands: []Band{{0.0, 0.2, 1, 1}, {0.25, 0.6, 0, 1}}}},
		{"negative edge", Spec{NumTaps: 21, Bands: []Band{{-0.1, 0.2, 1, 1}, {0.25, 0.5, 0, 1}}}},
		{"band runs backwards", Spec{NumTaps: 21, Bands: []Band{{0.3, 0.2, 1, 1}, {0.25, 0.5, 0, 1}}}},
		{"bands out of order", Spec{NumTaps: 21, Bands: []Band{{0.25, 0.5, 0, 1}, {0.0, 0.2, 1, 1}}}},
		{"bands touch", Spec{NumTaps: 21, Bands: []Band{{0.0, 0.2, 1, 1}, {0.2, 0.5, 0, 1}}}},
		{"bands overlap", Spec{NumTaps: 21, Bands: []Band{{0.0, 0.3, 1, 1}, {0.25, 0.5, 0, 1}}}},
		{"zero width band", Spec{NumTaps: 21, Bands: []Band{{0.2, 0.2, 1, 1}, {0.25, 0.5, 0, 1}}}},
		{"zero weight", Spec{NumTaps: 21, Bands: []Band{{0.0, 0.2, 1, 1}, {0.25, 0.5, 0, 0}}}},
		{"negative weight", Spec{NumTaps: 21, Bands: []Band{{0.0, 0.2, 1, 1}, {0.25, 0.5, 0, -1}}}},
		{"negative sample rate", Spec{NumTaps: 21, Bands: ok, SampleRate: -2000}},
		{"edge above nyquist in hz", Spec{NumTaps: 21, SampleRate: 2000,
			Bands: []Band{{0, 400, 1, 1}, {500, 1200, 0, 1}}}},
		{"bands too narrow for the taps", Spec{NumTaps: 6,
			Bands: []Band{{0.0, 0.01, 1, 21}, {0.06666666666666667, 0.0875, 0, 0.02857142857142857}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, err := Design(tt.spec)
			if err == nil {
				t.Fatalf("expected an error, got h=%v", h)
			}
			if h != nil {
				t.Errorf("expected no coefficients alongside the error, got %v", h)
			}
		})
	}
}

// TestDesignSampleRate checks that band edges given in Hz against a sample rate
// describe the same filter as the equivalent normalized edges.
func TestDesignSampleRate(t *testing.T) {
	const sr = 2000.0
	want, wantDev, err := Design(Spec{
		NumTaps: 21,
		Bands:   []Band{{0, 400 / sr, 1, 1}, {500 / sr, 0.5, 0, 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, gotDev, err := Design(Spec{
		NumTaps:    21,
		SampleRate: sr,
		Bands:      []Band{{0, 400, 1, 1}, {500, sr / 2, 0, 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) || gotDev != wantDev {
		t.Errorf("Hz edges gave a different filter than the normalized ones:\n got %v\nwant %v", got, want)
	}
}

// TestDesignDefaults checks that a zero grid density or iteration limit
// selects the reference program's LGRID of 16 and ITRMAX of 25.
func TestDesignDefaults(t *testing.T) {
	bands := []Band{{0.0, 0.2, 1.0, 1.0}, {0.25, 0.5, 0.0, 1.0}}

	want, wantDev, err := Design(Spec{NumTaps: 21, Bands: bands,
		GridDensity: defaultGridDensity, MaxIter: defaultMaxIterations})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ maxiter, density int }{{0, 0}, {-1, -1}} {
		got, gotDev, err := Design(Spec{NumTaps: 21, Bands: bands,
			GridDensity: tt.density, MaxIter: tt.maxiter})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) || gotDev != wantDev {
			t.Errorf("MaxIter=%d GridDensity=%d did not select the defaults", tt.maxiter, tt.density)
		}
	}
}

func FuzzDesign(f *testing.F) {
	f.Add(31, 0.0, 0.2, 0.25, 0.5, 1.0, 1.0, 0)
	f.Add(32, 0.0, 0.1, 0.2, 0.35, 10.0, 1.0, 0)
	f.Add(3, 0.05, 0.2, 0.25, 0.5, 1.0, 1.0, 1)
	f.Add(65, 0.0, 0.2, 0.25, 0.45, 100.0, 1.0, 2)
	// Two degenerate specs this target found. The first puts a single grid
	// point in the first band, leaving the grid shorter than the number of
	// extremal frequencies the exchange needs; the second drives the
	// deviation to around 1e-25, from which the coefficient recurrence
	// overflows. Both must be reported rather than approximated.
	f.Add(6, 0.0, 0.01, 0.06666666666666667, 0.0875, 21.0, 0.02857142857142857, 0)
	f.Add(65, 0.0, 0.0006666666666666666, 0.25, 0.45, 100.0, 3.0, 1)

	f.Fuzz(func(t *testing.T, numtaps int, e0, e1, e2, e3, w0, w1 float64, ftype int) {
		if numtaps < 3 || numtaps > 512 {
			return
		}
		edges := []float64{e0, e1, e2, e3}
		for _, e := range edges {
			if math.IsNaN(e) || e < 0 || e > 0.5 {
				return
			}
		}
		for i := 1; i < len(edges); i++ {
			if edges[i] <= edges[i-1] {
				return
			}
		}
		if math.IsNaN(w0) || math.IsNaN(w1) || w0 <= 0 || w1 <= 0 ||
			math.IsInf(w0, 0) || math.IsInf(w1, 0) {
			return
		}
		ft := FilterType(((ftype%3)+3)%3 + int(BandPass))

		h, dev, err := Design(Spec{
			NumTaps: numtaps,
			Type:    ft,
			Bands: []Band{
				{Lower: e0, Upper: e1, Response: 1.0, Weight: w0},
				{Lower: e2, Upper: e3, Response: 0.0, Weight: w1},
			},
		})
		switch {
		case err == nil:
		case errors.Is(err, ErrMaxIterations), errors.Is(err, ErrDeviationDecreased):
			// Documented to return the design it reached, so the checks
			// below still apply.
			if h == nil {
				t.Fatalf("%v returned no coefficients", err)
			}
		default:
			// A rejected input, or one the algorithm cannot make sense of.
			if h != nil {
				t.Fatalf("error %v came with %d coefficients", err, len(h))
			}
			return
		}

		// Finiteness is deliberately not asserted. Neither this package nor the
		// reference promises a usable filter for an arbitrary spec: a band
		// narrow enough to contribute one grid point drives the deviation down
		// to around 1e-25, from where the coefficient recurrence can overflow,
		// silently in the reference too. The caller's check is the returned
		// deviation, not the coefficients. What must hold for every input is
		// the result's return length.
		if len(h) != numtaps {
			t.Fatalf("got %d taps, want %d", len(h), numtaps)
		}
		if dev < 0 {
			t.Fatalf("deviation is negative: %v", dev)
		}
		sign := 1.0
		if ft != BandPass {
			sign = -1.0
		}
		for i := range numtaps {
			k := numtaps - 1 - i
			if h[k] != sign*h[i] && !(math.IsNaN(h[k]) && math.IsNaN(h[i])) {
				t.Fatalf("h[%d] = %v, not %v times h[%d] = %v", k, h[k], sign, i, h[i])
			}
		}
		if ft != BandPass && numtaps%2 == 1 && h[numtaps/2] != 0 {
			t.Fatalf("antisymmetric odd-length filter has h[%d] = %v, want 0", numtaps/2, h[numtaps/2])
		}
	})
}

func BenchmarkDesign(b *testing.B) {
	spec := Spec{
		NumTaps: 32,
		Bands: []Band{
			{0.0, 0.1, 0.0, 10.0},
			{0.2, 0.35, 1.0, 1.0},
			{0.425, 0.5, 0.0, 10.0},
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := Design(spec); err != nil {
			b.Fatal(err)
		}
	}
}

// TestDesignRejectsUnboundedSizes checks the guards on the two numbers the
// grid arrays are sized from.
//
// NumTaps and GridDensity are multiplied to size four slices, so either one
// alone can look modest while the product overflows an int -- after which make
// is handed a negative length and the package panics instead of reporting a
// specification it cannot serve.
func TestDesignRejectsUnboundedSizes(t *testing.T) {
	band := []Band{
		{Lower: 0, Upper: 0.2, Response: 1, Weight: 1},
		{Lower: 0.3, Upper: 0.5, Response: 0, Weight: 1},
	}
	for _, tc := range []struct {
		name string
		spec Spec
	}{
		{"huge NumTaps", Spec{NumTaps: math.MaxInt, Bands: band}},
		{"huge GridDensity", Spec{NumTaps: 31, GridDensity: math.MaxInt, Bands: band}},
		{"huge product", Spec{NumTaps: maxNumTaps, GridDensity: maxGridDensity, Bands: band}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Design(tc.spec); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestDesignRejectsNaN checks that a NaN in the specification is reported
// rather than spread through the exchange.
//
// Every comparison against a NaN is false, so it passes a "weight <= 0" test
// and an "edge outside [0, 0.5]" test alike. What comes back then is a filter
// of NaNs and a deviation of NaN, with no error to say why.
func TestDesignRejectsNaN(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		name  string
		bands []Band
	}{
		{"weight", []Band{{0, 0.2, 1, nan}, {0.3, 0.5, 0, 1}}},
		{"response", []Band{{0, 0.2, nan, 1}, {0.3, 0.5, 0, 1}}},
		{"lower edge", []Band{{nan, 0.2, 1, 1}, {0.3, 0.5, 0, 1}}},
		{"upper edge", []Band{{0, nan, 1, 1}, {0.3, 0.5, 0, 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, dev, err := Design(Spec{NumTaps: 31, Bands: tc.bands})
			if err == nil {
				t.Fatalf("no error; got deviation %v and taps %v", dev, h[:min(4, len(h))])
			}
		})
	}
}
