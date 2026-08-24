package dspviz

import (
	"fmt"
	"math"
)

// SpectralStats are the numbers read off an averaged spectrum.
type SpectralStats struct {
	Frames       int     `json:"frames"`
	NoiseBW      float64 `json:"noiseBW"`
	NoiseFloorDB float64 `json:"noiseFloorDB"`
	Flatness     float64 `json:"flatness"`

	PeakHz float64 `json:"peakHz"`
	PeakDB float64 `json:"peakDB"`
	SNRDB  float64 `json:"snrDB"`

	OccupiedLoHz float64 `json:"occupiedLoHz"`
	OccupiedHiHz float64 `json:"occupiedHiHz"`
	Width3DB     float64 `json:"width3DB"`
	Width20DB    float64 `json:"width20DB"`
	Width60DB    float64 `json:"width60DB"`

	// ImageRejectionDB is how far below the strongest tone its mirror sits, for
	// a two-sided spectrum. It is the same defect the quadrature figures
	// describe, measured a second and independent way.
	ImageRejectionDB float64 `json:"imageRejectionDB,omitempty"`

	Notes []string `json:"notes,omitempty"`
}

// SpectralStatsOf reads the spectral figures off an averaged spectrum.
func SpectralStatsOf(p *PSD) *SpectralStats {
	if p == nil || len(p.Freq) == 0 {
		return nil
	}
	s := &SpectralStats{
		Frames:       p.Frames,
		NoiseBW:      p.NoiseBW,
		NoiseFloorDB: p.NoiseFloorDB(20),
		Flatness:     p.Flatness(),
	}
	// A two-sided spectrum's DC bin holds the receiver's own offset rather than
	// anything that was transmitted, so it is not a candidate for the peak.
	skip := 0
	if p.TwoSided() {
		skip = 2
	}
	bin, db := p.PeakBin(skip)
	if bin >= 0 {
		s.PeakHz, s.PeakDB = p.Freq[bin], db
		s.SNRDB = db - s.NoiseFloorDB
	}
	s.OccupiedLoHz, s.OccupiedHiHz = OccupiedBandwidth(p, 0.99)
	lo, hi := BandwidthAtDB(p, 3)
	s.Width3DB = hi - lo
	lo, hi = BandwidthAtDB(p, 20)
	s.Width20DB = hi - lo
	lo, hi = BandwidthAtDB(p, 60)
	s.Width60DB = hi - lo

	if p.TwoSided() && bin >= 0 {
		n := len(p.Freq)
		if mirror := n - bin; mirror > 0 && mirror < n {
			s.ImageRejectionDB = p.AvgDB[mirror] - db
		}
		if sym, mean := conjugateSymmetric(p); sym {
			s.Notes = append(s.Notes, fmt.Sprintf(
				"the two sides of this spectrum agree to %.2f dB: the samples are really a real signal, "+
					"or I and Q hold the same thing", mean))
		}
	}
	return s
}

// mirrorSymmetric reports whether a spectrum equals itself under mirror, and by
// how much on average.
//
// Bins at the floor are left out: two silent bins agree perfectly, so counting
// them would make any quiet spectrum look like a mirror of itself.
func mirrorSymmetric(db []float64, floor float64, mirror func(int) int) (bool, float64) {
	n := len(db)
	var sum float64
	var count int
	for i := range n {
		j := mirror(i)
		if j <= i || j >= n {
			continue
		}
		if db[i] <= floor && db[j] <= floor {
			continue
		}
		sum += math.Abs(db[i] - db[j])
		count++
	}
	if count < 8 {
		return false, 0
	}
	mean := sum / float64(count)
	return mean < 1, mean
}

// conjugateSymmetric reports whether a two-sided spectrum is its own mirror,
// which a genuinely complex signal is not: its negative frequencies are signal
// rather than a reflection.
func conjugateSymmetric(p *PSD) (bool, float64) {
	n := len(p.AvgDB)
	if n < 8 {
		return false, 0
	}
	return mirrorSymmetric(p.AvgDB, p.Floor, func(i int) int { return n - i })
}
