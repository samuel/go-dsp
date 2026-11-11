//go:build !amd64 && !arm && !arm64 && (!386 || 386.softfloat)

package pcm

// The encode kernels where there is no assembly for them: every architecture
// but 386, amd64, arm and arm64, and 386 built with GO386=softfloat, whose CPU
// may lack SSE2. Each is its Go reference.

func f32ToU8(dst []byte, src []float32)    { floatToU8Ref(dst, src) }
func f64ToU8(dst []byte, src []float64)    { floatToU8Ref(dst, src) }
func f32ToI8(dst []byte, src []float32)    { floatToI8Ref(dst, src) }
func f64ToI8(dst []byte, src []float64)    { floatToI8Ref(dst, src) }
func f32ToI16LE(dst []byte, src []float32) { floatToI16LERef(dst, src) }
func f64ToI16LE(dst []byte, src []float64) { floatToI16LERef(dst, src) }
func f32ToI24LE(dst []byte, src []float32) { floatToI24LERef(dst, src) }
func f64ToI24LE(dst []byte, src []float64) { floatToI24LERef(dst, src) }
func f32ToI32LE(dst []byte, src []float32) { floatToI32LERef(dst, src) }
func f64ToI32LE(dst []byte, src []float64) { floatToI32LERef(dst, src) }
func f32ToF32LE(dst []byte, src []float32) { floatToF32LERef(dst, src) }
func f64ToF32LE(dst []byte, src []float64) { floatToF32LERef(dst, src) }
func f32ToF64LE(dst []byte, src []float32) { floatToF64LERef(dst, src) }
func f64ToF64LE(dst []byte, src []float64) { floatToF64LERef(dst, src) }
