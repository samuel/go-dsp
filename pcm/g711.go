package pcm

import "math/bits"

// G.711 companding.
//
// A code has a sign bit, a 3-bit segment and a 4-bit step within the segment.
// The steps of each segment are twice as wide as those of the segment below.
// A step's level is the value at its center, and the boundaries between steps
// are the decision values. Expanding a code returns its level. Compressing a
// sample finds the step its magnitude falls in.
//
// Levels are on the 16-bit scale, as in other G.711 decoders, so a decoded
// sample is a level divided by 2^15.

var (
	alawLevels  = g711Levels(alawExpand)
	mulawLevels = g711Levels(mulawExpand)
)

func g711Levels(expand func(byte) int16) *[256]int16 {
	var t [256]int16
	for i := range t {
		t[i] = expand(byte(i))
	}
	return &t
}

// alawExpand returns the level of an A-law code. A stored code has its even
// bits inverted. A-law has no zero level: the smallest are +8 and -8.
func alawExpand(code byte) int16 {
	v := code ^ 0x55
	seg := int32(v&0x70) >> 4
	t := int32(v&0x0f)<<4 + 8
	if seg > 0 {
		t = (t + 0x100) << (seg - 1)
	}
	if v&0x80 == 0 {
		t = -t
	}
	return int16(t)
}

// mulawBias is added to a mu-law magnitude before it is compressed, which
// makes every segment start at a power of two.
const mulawBias = 0x84

// mulawExpand returns the level of a mu-law code. A stored code has every bit
// inverted. Mu-law has two zeros: 0xff is positive and 0x7f negative.
func mulawExpand(code byte) int16 {
	v := ^code
	seg := int32(v&0x70) >> 4
	t := (int32(v&0x0f)<<3+mulawBias)<<seg - mulawBias
	if v&0x80 != 0 {
		t = -t
	}
	return int16(t)
}

// alawCompress returns the A-law code of the step that holds q, a sample on
// the 16-bit scale. Zero is encoded as positive.
func alawCompress(q int32) byte {
	sign := byte(0x80)
	if q < 0 {
		sign = 0
		q = -q
	}
	a := uint32(min(q, 1<<15-1))
	var seg, step uint32
	if a < 0x100 {
		step = a >> 4
	} else {
		seg = uint32(bits.Len32(a)) - 8
		step = a >> (seg + 3) & 0x0f
	}
	return (sign | byte(seg<<4) | byte(step)) ^ 0x55
}

// mulawCompress returns the mu-law code of the step that holds q, a sample on
// the 16-bit scale. A negative sample that rounds to zero is encoded as
// negative zero.
func mulawCompress(q int32) byte {
	var sign byte
	if q < 0 {
		sign = 0x80
		q = -q
	}
	// Clip to the top of the last step. A larger magnitude, once biased,
	// would need a ninth segment, and there are only eight.
	b := uint32(min(q, 1<<15-1-mulawBias)) + mulawBias
	seg := uint32(bits.Len32(b)) - 8
	step := b >> (seg + 3) & 0x0f
	return ^(sign | byte(seg<<4) | byte(step))
}

func alawToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = F(alawLevels[v]) * (1.0 / (1 << 15))
	}
}

func mulawToFloatRef[F float](dst []F, src []byte) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = F(mulawLevels[v]) * (1.0 / (1 << 15))
	}
}

// The encoders round to 16 bits the way floatToI16LERef does, then compress
// the result.

func floatToALawRef[F float](dst []byte, src []F) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = alawCompress(quantize(float64(v)*(1<<15), -(1 << 15), 1<<15-1))
	}
}

func floatToMuLawRef[F float](dst []byte, src []F) {
	n := min(len(dst), len(src))
	for i, v := range src[:n] {
		dst[i] = mulawCompress(quantize(float64(v)*(1<<15), -(1 << 15), 1<<15-1))
	}
}
