package convert_test

import (
	"fmt"
	"math"

	"samuelks.com/dsp"
	"samuelks.com/dsp/convert"
)

// Unsigned 8-bit samples, such as an rtl-sdr's, lose their 128 bias in the
// conversion but keep their scale. The scale to [-1, 1) belongs to whatever
// comes next; dsp.VScale is the fallback when nothing downstream can absorb
// it.
func ExampleU8ToF32() {
	raw := []byte{0, 64, 128, 192, 255}
	f := make([]float32, len(raw))
	convert.U8ToF32(f, raw)
	fmt.Println(f)

	dsp.VScale(f, f, 1.0/128)
	fmt.Println(f)
	// Output:
	// [-128 -64 0 64 127]
	// [-1 -0.5 0 0.5 0.9921875]
}

// F32ToI16 expects samples already at 16-bit full scale. Anything past it
// clips to the rails rather than wrapping, and NaN becomes zero.
func ExampleF32ToI16() {
	f := []float32{-40000, -32768, -0.5, 0.5, 1000.4, 32767, 40000, float32(math.NaN())}
	i := make([]int16, len(f))
	convert.F32ToI16(i, f)
	fmt.Println(i)
	// Output:
	// [-32768 -32768 0 0 1000 32767 32767 0]
}
