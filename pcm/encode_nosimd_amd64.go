//go:build !goexperiment.simd

package pcm

// Without GOEXPERIMENT=simd the vector kernel is the generated assembly,
// dispatched on the CPU's features in pcm_asm_amd64.go.

func f32ToU8(dst []byte, src []float32)    { floatToU8Ref(dst, src) }
func f64ToU8(dst []byte, src []float64)    { floatToU8Ref(dst, src) }
func f32ToI8(dst []byte, src []float32)    { floatToI8Ref(dst, src) }
func f64ToI8(dst []byte, src []float64)    { floatToI8Ref(dst, src) }
func f32ToI16LE(dst []byte, src []float32) { f32ToI16LEAsm(dst, src) }
func f64ToI16LE(dst []byte, src []float64) { floatToI16LERef(dst, src) }
func f32ToI24LE(dst []byte, src []float32) { floatToI24LERef(dst, src) }
func f64ToI24LE(dst []byte, src []float64) { floatToI24LERef(dst, src) }
func f32ToI32LE(dst []byte, src []float32) { floatToI32LERef(dst, src) }
func f64ToI32LE(dst []byte, src []float64) { floatToI32LERef(dst, src) }
func f32ToF32LE(dst []byte, src []float32) { floatToF32LERef(dst, src) }
func f64ToF32LE(dst []byte, src []float64) { floatToF32LERef(dst, src) }
func f32ToF64LE(dst []byte, src []float32) { floatToF64LERef(dst, src) }
func f64ToF64LE(dst []byte, src []float64) { floatToF64LERef(dst, src) }
