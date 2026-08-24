//go:build !arm && !arm64

package encoding

import "github.com/samuel/go-dsp/dsp/internal/view"

// U8ToC64 converts unsigned 8-bit interleaved complex samples to complex64
// (32-bit parts), subtracting the 128 bias. It does not scale.
//
// Interleaved u8 I/Q to complex64 is the same memory operation as u8 to float32
// with twice as many outputs, so this reslices into U8ToF32 and inherits the
// implementation active there.
func U8ToC64(dst []complex64, src []byte) {
	n := min(len(src)/2, len(dst))
	U8ToF32(view.C64ToF32(dst)[:n*2], src[:n*2])
}
