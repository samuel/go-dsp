package dsp

// Interleaved and planar layouts.

// interleaveChannelOuterMax is the channel count up to which Interleave writes
// one channel at a time. It measures the same at float32 and float64, so the
// cost is the number of passes rather than the byte stride.
const interleaveChannelOuterMax = 4

// Deinterleave writes each channel of an interleaved buffer into its own
// contiguous plane and returns the number of frames written to each. The
// channel count is len(dst) and the frame count is
// min(len(src)/channels, shortest plane); nothing past it is touched. The
// planes must not overlap one another or src.
func Deinterleave[T Sample](dst [][]T, src []T) int {
	channels := len(dst)
	if channels == 0 {
		return 0
	}
	frames := len(src) / channels
	for _, p := range dst {
		frames = min(frames, len(p))
	}
	if frames <= 0 {
		return 0
	}
	for c, p := range dst {
		p = p[:frames]
		src := src[c:]
		for i := range p {
			p[i] = src[i*channels]
		}
	}
	return frames
}

// Interleave writes one plane per channel into an interleaved buffer and
// returns the number of frames written. It is Deinterleave's inverse, with the
// same channel count, frame count and overlap rules.
func Interleave[T Sample](dst []T, src [][]T) int {
	channels := len(src)
	if channels == 0 {
		return 0
	}
	frames := len(dst) / channels
	for _, p := range src {
		frames = min(frames, len(p))
	}
	if frames <= 0 {
		return 0
	}
	out := dst[:frames*channels]
	if channels <= interleaveChannelOuterMax {
		for c, p := range src {
			p = p[:frames]
			dst := out[c:]
			for i, v := range p {
				dst[i*channels] = v
			}
		}
		return frames
	}
	for i := range frames {
		f := out[i*channels:][:channels]
		for c, p := range src {
			f[c] = p[i]
		}
	}
	return frames
}
