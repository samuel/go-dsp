//go:build !goexperiment.simd

package pcm

// The U8, I8 and I16LE kernels are the NEON assembly in pcm_nosimd_arm64.s;
// the rest are the references.

//go:noescape
func u8ToF32(dst []float32, src []byte)

//go:noescape
func i8ToF32(dst []float32, src []byte)

//go:noescape
func i16LEToF32(dst []float32, src []byte)

func u8ToF64(dst []float64, src []byte)    { u8ToFloatRef(dst, src) }
func i8ToF64(dst []float64, src []byte)    { i8ToFloatRef(dst, src) }
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
