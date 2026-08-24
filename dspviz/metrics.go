package dspviz

import "math"

// BandMetrics summarizes a response over one band of frequencies, in dB.
type BandMetrics struct {
	MaxDB    float64 // the highest magnitude in the band
	MinDB    float64 // the lowest
	RippleDB float64 // MaxDB - MinDB, the peak-to-peak variation
	Points   int     // how many grid points fell in the band
}

// Metrics are the numbers worth reading off a response, as against looking at.
type Metrics struct {
	// CutoffHz is where the magnitude falls 3.0103 dB below its passband peak,
	// interpolated between grid points rather than snapped to one of them.
	// Zero means the response never crosses.
	CutoffHz float64

	// StopEdgeHz is the lowest frequency at or above CutoffHz where the
	// magnitude first reaches the stopband's peak. TransitionHz is the distance
	// from CutoffHz to it.
	StopEdgeHz   float64
	TransitionHz float64

	Passband BandMetrics
	Stopband BandMetrics

	// GroupDelaySamples is the mean group delay across the passband. For a
	// linear-phase filter it is the whole story; for anything else compare it
	// against the spread the plot shows.
	GroupDelaySamples float64
}

// Measure reads the numbers off a response. passband and stopband are
// {low, high} in Hz; an empty stopband (high not above low) leaves those
// figures zero.
func Measure(r *Response, passband, stopband [2]float64) Metrics {
	var m Metrics
	db := r.MagnitudeDB()

	m.Passband = bandMetrics(r.Freq, db, passband)
	m.Stopband = bandMetrics(r.Freq, db, stopband)

	peak := m.Passband.MaxDB
	if m.Passband.Points == 0 {
		peak = math.Inf(-1)
		for _, v := range db {
			peak = math.Max(peak, v)
		}
	}

	// The -3 dB point, interpolated in dB against the logarithm of frequency.
	// Snapping to the nearest grid point instead makes the headline number as
	// coarse as the grid, which for a log grid near DC is very coarse indeed.
	target := peak - 3.0102999566398
	for i := 1; i < len(db); i++ {
		if db[i-1] >= target && db[i] < target {
			m.CutoffHz = interpLogF(r.Freq[i-1], r.Freq[i], db[i-1], db[i], target)
			break
		}
	}

	if m.Stopband.Points > 0 && m.CutoffHz > 0 {
		for i := range db {
			if r.Freq[i] >= m.CutoffHz && db[i] <= m.Stopband.MaxDB {
				m.StopEdgeHz = r.Freq[i]
				m.TransitionHz = m.StopEdgeHz - m.CutoffHz
				break
			}
		}
	}

	if d := r.GroupDelay(); len(d) > 0 {
		var sum float64
		var n int
		for i, f := range r.Freq {
			if inBand(f, passband) {
				sum += d[i]
				n++
			}
		}
		if n > 0 {
			m.GroupDelaySamples = sum / float64(n)
		}
	}
	return m
}

func bandMetrics(freq, db []float64, band [2]float64) BandMetrics {
	b := BandMetrics{MaxDB: math.Inf(-1), MinDB: math.Inf(1)}
	for i, f := range freq {
		if !inBand(f, band) {
			continue
		}
		b.MaxDB = math.Max(b.MaxDB, db[i])
		b.MinDB = math.Min(b.MinDB, db[i])
		b.Points++
	}
	if b.Points == 0 {
		return BandMetrics{}
	}
	b.RippleDB = b.MaxDB - b.MinDB
	return b
}

func inBand(f float64, band [2]float64) bool {
	return band[1] > band[0] && f >= band[0] && f <= band[1]
}

// interpLogF finds where a straight line through (f0, d0) and (f1, d1), with
// frequency on a logarithmic axis, crosses target.
func interpLogF(f0, f1, d0, d1, target float64) float64 {
	if d1 == d0 {
		return f0
	}
	t := (target - d0) / (d1 - d0)
	if f0 > 0 && f1 > 0 {
		return math.Exp(math.Log(f0) + t*(math.Log(f1)-math.Log(f0)))
	}
	return f0 + t*(f1-f0)
}

// DB converts a magnitude to decibels. A zero magnitude gives negative
// infinity, which callers clip at whatever floor their plot has.
func DB(magnitude float64) float64 { return dbAmplitude(magnitude) }

// Amplitude is the inverse of DB.
func Amplitude(db float64) float64 { return math.Pow(10, db/20) }

// dbAmplitude and dbPower are the two decibel conversions, named so a call
// site says which quantity it holds: an amplitude is twenty times the log, a
// power is ten.
func dbAmplitude(magnitude float64) float64 { return 20 * math.Log10(magnitude) }
func dbPower(power float64) float64         { return 10 * math.Log10(power) }

// floorOr is the requested floor, or def where the caller left it zero. Zero is
// how ChartOptions spells "choose one", so it cannot also mean 0 dBFS.
func floorOr(requested, def float64) float64 {
	if requested == 0 {
		return def
	}
	return requested
}

// floorFor is floorOr raised to leave a little headroom below the lowest value
// on the plot.
//
// FloorDB is a limit and not a target: a gentle filter may only fall a dozen
// decibels across its transition, and pinning the axis at -180 for it spends
// the whole pane on empty space and flattens the very slope the plot exists to
// show. The deeper of the two therefore wins.
func floorFor(requested, def, data float64) float64 {
	// Enough space under the trace that the lowest point is not on the frame.
	const padDB = 6
	return math.Max(floorOr(requested, def), data-padDB)
}
