package pcm

// The kernels with a body-less declaration are pcm_arm.s, NEON or VFP by
// haveNEON; the rest are the Go references.

func f32ToU8(dst []byte, src []float32) { floatToU8Ref(dst, src) }
func f64ToU8(dst []byte, src []float64) { floatToU8Ref(dst, src) }
func f32ToI8(dst []byte, src []float32) { floatToI8Ref(dst, src) }
func f64ToI8(dst []byte, src []float64) { floatToI8Ref(dst, src) }

//go:noescape
func f32ToI16LE(dst []byte, src []float32)

func f64ToI16LE(dst []byte, src []float64) { floatToI16LERef(dst, src) }
func f32ToI24LE(dst []byte, src []float32) { floatToI24LERef(dst, src) }
func f64ToI24LE(dst []byte, src []float64) { floatToI24LERef(dst, src) }
func f32ToI32LE(dst []byte, src []float32) { floatToI32LERef(dst, src) }
func f64ToI32LE(dst []byte, src []float64) { floatToI32LERef(dst, src) }
func f32ToF32LE(dst []byte, src []float32) { floatToF32LERef(dst, src) }
func f64ToF32LE(dst []byte, src []float64) { floatToF32LERef(dst, src) }
func f32ToF64LE(dst []byte, src []float32) { floatToF64LERef(dst, src) }
func f64ToF64LE(dst []byte, src []float64) { floatToF64LERef(dst, src) }
