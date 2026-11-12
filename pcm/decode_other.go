//go:build !amd64 && !arm && !arm64 && (!386 || 386.softfloat)

package pcm

// The decode kernels where there is no assembly for them: every architecture
// but 386, amd64, arm and arm64, and 386 built with GO386=softfloat, whose CPU
// may lack SSE2. Each is its Go reference.

func u8ToF32(dst []float32, src []byte)    { u8ToFloatRef(dst, src) }
func u8ToF64(dst []float64, src []byte)    { u8ToFloatRef(dst, src) }
func i8ToF32(dst []float32, src []byte)    { i8ToFloatRef(dst, src) }
func i8ToF64(dst []float64, src []byte)    { i8ToFloatRef(dst, src) }
func i16LEToF32(dst []float32, src []byte) { i16LEToFloatRef(dst, src) }
func i16LEToF64(dst []float64, src []byte) { i16LEToFloatRef(dst, src) }
func i24LEToF32(dst []float32, src []byte) { i24LEToFloatRef(dst, src) }
func i24LEToF64(dst []float64, src []byte) { i24LEToFloatRef(dst, src) }
func i32LEToF32(dst []float32, src []byte) { i32LEToFloatRef(dst, src) }
func i32LEToF64(dst []float64, src []byte) { i32LEToFloatRef(dst, src) }
func f32LEToF32(dst []float32, src []byte) { f32LEToFloatRef(dst, src) }
func f32LEToF64(dst []float64, src []byte) { f32LEToFloatRef(dst, src) }
func f64LEToF32(dst []float32, src []byte) { f64LEToFloatRef(dst, src) }
func f64LEToF64(dst []float64, src []byte) { f64LEToFloatRef(dst, src) }
func alawToF32(dst []float32, src []byte)  { alawToFloatRef(dst, src) }
func alawToF64(dst []float64, src []byte)  { alawToFloatRef(dst, src) }
func mulawToF32(dst []float32, src []byte) { mulawToFloatRef(dst, src) }
func mulawToF64(dst []float64, src []byte) { mulawToFloatRef(dst, src) }
