package dsp

import "math"

// http://www.musicdsp.org/files/Audio-EQ-Cookbook.txt

type BiQuadFilter struct {
	B0, B1, B2      float64
	A0, A1, A2      float64
	prevIn, prevOut [2]float64
}

func (f *BiQuadFilter) Filter(input, output []float64) {
	b0a0 := f.B0 / f.A0
	b1a0 := f.B1 / f.A0
	b2a0 := f.B2 / f.A0
	a1a0 := f.A1 / f.A0
	a2a0 := f.A2 / f.A0
	for i, s := range input {
		newSample := b0a0*s + b1a0*f.prevIn[0] + b2a0*f.prevIn[1] - a1a0*f.prevOut[0] - a2a0*f.prevOut[1]
		f.prevOut[1] = f.prevOut[0]
		f.prevOut[0] = newSample
		f.prevIn[1] = f.prevIn[0]
		f.prevIn[0] = s
		output[i] = newSample
	}
}

func (f *BiQuadFilter) FilterF32(input, output []float32) {
	b0a0 := f.B0 / f.A0
	b1a0 := f.B1 / f.A0
	b2a0 := f.B2 / f.A0
	a1a0 := f.A1 / f.A0
	a2a0 := f.A2 / f.A0
	for i, s := range input {
		newSample := b0a0*float64(s) + b1a0*f.prevIn[0] + b2a0*f.prevIn[1] - a1a0*f.prevOut[0] - a2a0*f.prevOut[1]
		f.prevOut[1] = f.prevOut[0]
		f.prevOut[0] = newSample
		f.prevIn[1] = f.prevIn[0]
		f.prevIn[0] = float64(s)
		output[i] = float32(newSample)
	}
}

// H(s) = 1 / (s^2 + s/Q + 1)
func NewLowPassBiQuadFilter(sampleRate, cutoffFreq, q float64) *BiQuadFilter {
	w0 := 2 * math.Pi * cutoffFreq / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	return &BiQuadFilter{
		B0: (1 - cosW0) / 2,
		B1: 1 - cosW0,
		B2: (1 - cosW0) / 2,
		A0: 1 + alpha,
		A1: -2 * cosW0,
		A2: 1 - alpha,
	}
}

// H(s) = s^2 / (s^2 + s/Q + 1)
func NewHighPassBiQuadFilter(sampleRate, cutoffFreq, q float64) *BiQuadFilter {
	w0 := 2 * math.Pi * cutoffFreq / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	return &BiQuadFilter{
		B0: (1 + cosW0) / 2,
		B1: -(1 + cosW0),
		B2: (1 + cosW0) / 2,
		A0: 1 + alpha,
		A1: -2 * cosW0,
		A2: 1 - alpha,
	}
}

// H(s) = s / (s^2 + s/Q + 1)  (constant skirt gain, peak gain = Q)
func NewBandPassConstantSkirtGainBiQuadFilter(sampleRate, centreFrequency, q float64) *BiQuadFilter {
	w0 := 2 * math.Pi * centreFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	return &BiQuadFilter{
		B0: sinW0 / 2, // = Q*alpha
		B1: 0,
		B2: -sinW0 / 2, // = -Q*alpha
		A0: 1 + alpha,
		A1: -2 * cosW0,
		A2: 1 - alpha,
	}
}

// H(s) = (s/Q) / (s^2 + s/Q + 1)      (constant 0 dB peak gain)
func NewBandPassConstantPeakGainBiQuadFilter(sampleRate, centreFrequency, q float64) *BiQuadFilter {
	w0 := 2 * math.Pi * centreFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	return &BiQuadFilter{
		B0: alpha,
		B1: 0,
		B2: -alpha,
		A0: 1 + alpha,
		A1: -2 * cosW0,
		A2: 1 - alpha,
	}
}

// H(s) = (s^2 + 1) / (s^2 + s/Q + 1)
func NotchBiQuadFilter(sampleRate, centreFrequency, q float64) *BiQuadFilter {
	w0 := 2 * math.Pi * centreFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	return &BiQuadFilter{
		B0: 1,
		B1: -2 * cosW0,
		B2: 1,
		A0: 1 + alpha,
		A1: -2 * cosW0,
		A2: 1 - alpha,
	}
}

// H(s) = (s^2 - s/Q + 1) / (s^2 + s/Q + 1)
func AllPassBiQuadFilter(sampleRate, centreFrequency, q float64) *BiQuadFilter {
	w0 := 2 * math.Pi * centreFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	return &BiQuadFilter{
		B0: 1 - alpha,
		B1: -2 * cosW0,
		B2: 1 + alpha,
		A0: 1 + alpha,
		A1: -2 * cosW0,
		A2: 1 - alpha,
	}
}

// H(s) = (s^2 + s*(A/Q) + 1) / (s^2 + s/(A*Q) + 1)
func PeakingEQBiQuadFilter(sampleRate, centreFrequency, q, dbGain float64) *BiQuadFilter {
	w0 := 2 * math.Pi * centreFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	alpha := sinW0 / (2 * q)
	a := math.Pow(10, dbGain/40) // TODO: should we square root this value?
	return &BiQuadFilter{
		B0: 1 + alpha*a,
		B1: -2 * cosW0,
		B2: 1 - alpha*a,
		A0: 1 + alpha/a,
		A1: -2 * cosW0,
		A2: 1 - alpha/a,
	}
}

// H(s) = A * (s^2 + (sqrt(A)/Q)*s + A)/(A*s^2 + (sqrt(A)/Q)*s + 1)
// shelfSlope: a "shelf slope" parameter (for shelving EQ only).
// When S = 1, the shelf slope is as steep as it can be and remain monotonically
// increasing or decreasing gain with frequency.  The shelf slope, in dB/octave,
// remains proportional to S for all other values for a fixed f0/Fs and dBgain.</param>
// dbGain: Gain in decibels
func LowShelfBiQuadFilter(sampleRate, cutoffFrequency, shelfSlope, dbGain float64) *BiQuadFilter {
	w0 := 2 * math.Pi * cutoffFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	a := math.Pow(10, dbGain/40.0) // TODO: should we square root this value?
	alpha := sinW0 / 2 * math.Sqrt((a+1/a)*(1/shelfSlope-1)+2)
	temp := 2 * math.Sqrt(a) * alpha
	return &BiQuadFilter{
		B0: a * ((a + 1) - (a-1)*cosW0 + temp),
		B1: 2 * a * ((a - 1) - (a+1)*cosW0),
		B2: a * ((a + 1) - (a-1)*cosW0 - temp),
		A0: (a + 1) + (a-1)*cosW0 + temp,
		A1: -2 * ((a - 1) + (a+1)*cosW0),
		A2: (a + 1) + (a-1)*cosW0 - temp,
	}
}

// H(s) = A * (A*s^2 + (sqrt(A)/Q)*s + 1)/(s^2 + (sqrt(A)/Q)*s + A)
func HighShelfBiQuadFilter(sampleRate, cutoffFrequency, shelfSlope, dbGain float64) *BiQuadFilter {
	w0 := 2 * math.Pi * cutoffFrequency / sampleRate
	sinW0 := math.Sin(w0)
	cosW0 := math.Cos(w0)
	a := math.Pow(10, dbGain/40) // TODO: should we square root this value?
	alpha := sinW0 / 2 * math.Sqrt((a+1/a)*(1/shelfSlope-1)+2)
	temp := 2 * math.Sqrt(a) * alpha
	return &BiQuadFilter{
		B0: a * ((a + 1) + (a-1)*cosW0 + temp),
		B1: -2 * a * ((a - 1) + (a+1)*cosW0),
		B2: a * ((a + 1) + (a-1)*cosW0 - temp),
		A0: (a + 1) - (a-1)*cosW0 + temp,
		A1: 2 * ((a - 1) - (a+1)*cosW0),
		A2: (a + 1) - (a-1)*cosW0 - temp,
	}
}
