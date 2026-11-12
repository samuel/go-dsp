//go:build !goexperiment.simd

package pcm

// The float32 to I16LE kernel is the NEON assembly in pcm_nosimd_arm64.s; the
// rest are the references.

//go:noescape
func f32ToI16LE(dst []byte, src []float32)

func f32ToU8(dst []byte, src []float32)    { floatToU8Ref(dst, src) }
func f64ToU8(dst []byte, src []float64)    { floatToU8Ref(dst, src) }
func f32ToI8(dst []byte, src []float32)    { floatToI8Ref(dst, src) }
func f64ToI8(dst []byte, src []float64)    { floatToI8Ref(dst, src) }
func f64ToI16LE(dst []byte, src []float64) { floatToI16LERef(dst, src) }
func f32ToI24LE(dst []byte, src []float32) { floatToI24LERef(dst, src) }
func f64ToI24LE(dst []byte, src []float64) { floatToI24LERef(dst, src) }
func f32ToI32LE(dst []byte, src []float32) { floatToI32LERef(dst, src) }
func f64ToI32LE(dst []byte, src []float64) { floatToI32LERef(dst, src) }
func f32ToF32LE(dst []byte, src []float32) { floatToF32LERef(dst, src) }
func f64ToF32LE(dst []byte, src []float64) { floatToF32LERef(dst, src) }
func f32ToF64LE(dst []byte, src []float32) { floatToF64LERef(dst, src) }
func f64ToF64LE(dst []byte, src []float64) { floatToF64LERef(dst, src) }
func f32ToALaw(dst []byte, src []float32)  { floatToALawRef(dst, src) }
func f64ToALaw(dst []byte, src []float64)  { floatToALawRef(dst, src) }
func f32ToMuLaw(dst []byte, src []float32) { floatToMuLawRef(dst, src) }
func f64ToMuLaw(dst []byte, src []float64) { floatToMuLawRef(dst, src) }
