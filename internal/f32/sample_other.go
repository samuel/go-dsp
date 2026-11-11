//go:build !arm

package f32

// FastAtan2 returns an approximation of Atan2(y, x); see dsp.FastAtan2.
func FastAtan2(y, x float32) float32 {
	return fastAtan2(y, x)
}

// FastAtan2Fine returns an approximation of Atan2(y, x); see dsp.FastAtan2Fine.
func FastAtan2Fine(y, x float32) float32 {
	return fastAtan2Fine(y, x)
}

// Rotate90 multiplies samples by 1, j, -1, -j in turn, over whole groups of
// four. A trailing group of fewer than four is left untouched.
func Rotate90(samples []complex64) {
	rotate90(samples)
}

// FMDemod writes the phase step from each sample to the next, starting from
// pre, over min(len(dst), len(src)) samples, and returns the last sample it
// read, or pre if it read none.
func FMDemod(dst []float32, src []complex64, pre complex64) complex64 {
	return fmDemod(dst, src, pre)
}

// BoxcarDecimate sums each group of factor samples into one output. sum and
// count are the partial group carried in, and it returns the number of
// outputs with the partial group to carry out. dst must have room for
// (count+len(src))/factor samples.
func BoxcarDecimate(dst, src []complex64, factor int, sum complex64, count int) (int, complex64, int) {
	return boxcarDecimate(dst, src, factor, sum, count)
}

// RationalBoxcarDecimate averages the samples falling in each output period
// of a decimation from rate fast to rate slow. sum, count and phase are the
// partial output carried in, and it returns the number of outputs with the
// partial output to carry out. dst must have room for
// (phase+len(src)*slow)/fast samples.
func RationalBoxcarDecimate(dst, src []float32, fast, slow int, sum float32, count, phase int) (int, float32, int, int) {
	return rationalBoxcarDecimate(dst, src, fast, slow, sum, count, phase)
}
