package pcm

import (
	"testing"

	"samuelks.com/dsp"
)

// benchSizes is the sample-count sweep. 1K-1 is not a multiple of any block,
// so it measures the remainder paths.
var benchSizes = []struct {
	name string
	n    int
}{
	{"64", 64}, {"256", 256}, {"1K", 1024}, {"1K-1", 1023},
	{"4K", 4096}, {"16K", 16384}, {"64K", 65536},
}

// BenchmarkKernel measures each kernel of the grid directly, at each size,
// with throughput in samples per second reported as MB/s of input bytes.
func BenchmarkKernel(b *testing.B) {
	for _, k := range decodeF32Kernels {
		b.Run(k.name, func(b *testing.B) { benchDecodeKernel(b, k.width, k.kernel) })
	}
	for _, k := range decodeF64Kernels {
		b.Run(k.name, func(b *testing.B) { benchDecodeKernel(b, k.width, k.kernel) })
	}
	for _, k := range encodeF32Kernels {
		b.Run(k.name, func(b *testing.B) { benchEncodeKernel(b, k.width, k.kernel) })
	}
	for _, k := range encodeF64Kernels {
		b.Run(k.name, func(b *testing.B) { benchEncodeKernel(b, k.width, k.kernel) })
	}
}

func benchDecodeKernel[F float](b *testing.B, width int, kernel func([]F, []byte)) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]byte, sz.n*width)
			for i := range src {
				src[i] = byte(i * 7)
			}
			dst := make([]F, sz.n)
			b.SetBytes(int64(len(src)))
			for b.Loop() {
				kernel(dst, src)
			}
		})
	}
}

func benchEncodeKernel[F float](b *testing.B, width int, kernel func([]byte, []F)) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]F, sz.n)
			for i := range src {
				src[i] = F(i%2001-1000) / 1000
			}
			dst := make([]byte, sz.n*width)
			b.SetBytes(int64(len(dst)))
			for b.Loop() {
				kernel(dst, src)
			}
		})
	}
}

// BenchmarkDecode and BenchmarkEncode measure the public functions, dispatch
// and big-endian reversal included, for a little- and a big-endian format at
// each sample type.
func BenchmarkDecode(b *testing.B) {
	for _, f := range []Format{U8, I16LE, I16BE, F32LE, ALaw, MuLaw} {
		b.Run(f.String(), func(b *testing.B) {
			b.Run("float32", func(b *testing.B) { benchDecode[float32](b, f) })
			b.Run("float64", func(b *testing.B) { benchDecode[float64](b, f) })
			b.Run("complex64", func(b *testing.B) { benchDecode[complex64](b, f) })
			b.Run("complex128", func(b *testing.B) { benchDecode[complex128](b, f) })
		})
	}
}

func BenchmarkEncode(b *testing.B) {
	for _, f := range []Format{U8, I16LE, I16BE, F32LE, ALaw, MuLaw} {
		b.Run(f.String(), func(b *testing.B) {
			b.Run("float32", func(b *testing.B) { benchEncode[float32](b, f) })
			b.Run("float64", func(b *testing.B) { benchEncode[float64](b, f) })
			b.Run("complex64", func(b *testing.B) { benchEncode[complex64](b, f) })
			b.Run("complex128", func(b *testing.B) { benchEncode[complex128](b, f) })
		})
	}
}

func benchDecode[T dsp.Sample](b *testing.B, f Format) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]byte, sz.n*f.Width()*elems[T]())
			for i := range src {
				src[i] = byte(i * 7)
			}
			dst := make([]T, sz.n)
			b.SetBytes(int64(len(src)))
			for b.Loop() {
				Decode(dst, src, f)
			}
		})
	}
}

func benchEncode[T dsp.Sample](b *testing.B, f Format) {
	for _, sz := range benchSizes {
		b.Run(sz.name, func(b *testing.B) {
			src := make([]T, sz.n)
			vals := make([]float64, sz.n*elems[T]())
			for i := range vals {
				vals[i] = float64(i%2001-1000) / 1000
			}
			dst := make([]byte, len(vals)*f.Width())
			Encode(dst, vals, F64LE)
			Decode(src, dst, F64LE)
			b.SetBytes(int64(len(dst)))
			for b.Loop() {
				Encode(dst, src, f)
			}
		})
	}
}
