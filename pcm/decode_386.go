//go:build !386.softfloat

package pcm

// The SSE2 kernels are in pcm_386.s. GO386=softfloat builds the references
// instead, since its target CPUs may lack SSE2.

//go:noescape
func u8ToF32(dst []float32, src []byte)

func u8ToF64(dst []float64, src []byte) { u8ToFloatRef(dst, src) }

//go:noescape
func i8ToF32(dst []float32, src []byte)

func i8ToF64(dst []float64, src []byte) { i8ToFloatRef(dst, src) }

//go:noescape
func i16LEToF32(dst []float32, src []byte)

func i16LEToF64(dst []float64, src []byte) { i16LEToFloatRef(dst, src) }
func i24LEToF32(dst []float32, src []byte) { i24LEToFloatRef(dst, src) }
func i24LEToF64(dst []float64, src []byte) { i24LEToFloatRef(dst, src) }
func i32LEToF32(dst []float32, src []byte) { i32LEToFloatRef(dst, src) }
func i32LEToF64(dst []float64, src []byte) { i32LEToFloatRef(dst, src) }
func f32LEToF32(dst []float32, src []byte) { f32LEToFloatRef(dst, src) }
func f32LEToF64(dst []float64, src []byte) { f32LEToFloatRef(dst, src) }
func f64LEToF32(dst []float32, src []byte) { f64LEToFloatRef(dst, src) }
func f64LEToF64(dst []float64, src []byte) { f64LEToFloatRef(dst, src) }
