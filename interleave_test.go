package dsp

import (
	"fmt"
	"testing"
)

// The sweeps run every channel count from one to eight and every frame count
// up to two prospective block sizes, at all four widths Sample admits, against
// a naive double loop written out here.

const deinterleaveMaxFrames = 40

// refDeinterleave and refInterleave are the definitions the sweeps compare
// against.
func refDeinterleave[T Sample](src []T, channels, frames int) [][]T {
	out := make([][]T, channels)
	for c := range out {
		out[c] = make([]T, frames)
		for i := range frames {
			out[c][i] = src[i*channels+c]
		}
	}
	return out
}

func refInterleave[T Sample](planes [][]T, frames int) []T {
	channels := len(planes)
	out := make([]T, frames*channels)
	for i := range frames {
		for c := range channels {
			out[i*channels+c] = planes[c][i]
		}
	}
	return out
}

func testDeinterleave[T Sample](t *testing.T, val func(int) T) {
	t.Helper()
	sentinel := val(-1)
	for channels := 1; channels <= 8; channels++ {
		for frames := range deinterleaveMaxFrames {
			src := make([]T, frames*channels)
			for i := range src {
				src[i] = val(i)
			}
			got := make([][]T, channels)
			for c := range got {
				got[c] = make([]T, frames)
				for i := range got[c] {
					got[c][i] = sentinel
				}
			}
			if n := Deinterleave(got, src); n != frames {
				t.Fatalf("%dch/%dframes: wrote %d frames, want %d", channels, frames, n, frames)
			}
			want := refDeinterleave(src, channels, frames)
			for c := range want {
				for i := range want[c] {
					if got[c][i] != want[c][i] {
						t.Fatalf("%dch/%dframes: channel %d frame %d = %v, want %v",
							channels, frames, c, i, got[c][i], want[c][i])
					}
				}
			}
		}
	}
}

func testInterleave[T Sample](t *testing.T, val func(int) T) {
	t.Helper()
	sentinel := val(-1)
	for channels := 1; channels <= 8; channels++ {
		for frames := range deinterleaveMaxFrames {
			planes := make([][]T, channels)
			for c := range planes {
				planes[c] = make([]T, frames)
				for i := range planes[c] {
					planes[c][i] = val(c*deinterleaveMaxFrames + i)
				}
			}
			got := make([]T, frames*channels)
			for i := range got {
				got[i] = sentinel
			}
			if n := Interleave(got, planes); n != frames {
				t.Fatalf("%dch/%dframes: wrote %d frames, want %d", channels, frames, n, frames)
			}
			want := refInterleave(planes, frames)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%dch/%dframes: element %d = %v, want %v",
						channels, frames, i, got[i], want[i])
				}
			}
		}
	}
}

// testRoundTrip catches a kernel that permutes correctly within a block and
// wrongly across one, which is how a lane-crossing implementation fails.
func testRoundTrip[T Sample](t *testing.T, val func(int) T) {
	t.Helper()
	for channels := 1; channels <= 8; channels++ {
		for frames := range deinterleaveMaxFrames {
			src := make([]T, frames*channels)
			for i := range src {
				src[i] = val(i)
			}
			planes := make([][]T, channels)
			for c := range planes {
				planes[c] = make([]T, frames)
			}
			if n := Deinterleave(planes, src); n != frames {
				t.Fatalf("%dch/%dframes: deinterleaved %d frames", channels, frames, n)
			}
			got := make([]T, frames*channels)
			if n := Interleave(got, planes); n != frames {
				t.Fatalf("%dch/%dframes: interleaved %d frames", channels, frames, n)
			}
			for i := range src {
				if got[i] != src[i] {
					t.Fatalf("%dch/%dframes: element %d came back %v, want %v",
						channels, frames, i, got[i], src[i])
				}
			}
		}
	}
}

// Separate generators per width because Go will not convert a non-constant int
// to a complex type.
func f32val(i int) float32     { return float32(i) * 0.5 }
func f64val(i int) float64     { return float64(i) * 0.5 }
func c64val(i int) complex64   { return complex(float32(i), float32(-i)) }
func c128val(i int) complex128 { return complex(float64(i), float64(-i)) }

func TestDeinterleave(t *testing.T) {
	t.Run("float32", func(t *testing.T) { testDeinterleave(t, f32val) })
	t.Run("float64", func(t *testing.T) { testDeinterleave(t, f64val) })
	t.Run("complex64", func(t *testing.T) { testDeinterleave(t, c64val) })
	t.Run("complex128", func(t *testing.T) { testDeinterleave(t, c128val) })
}

func TestInterleave(t *testing.T) {
	t.Run("float32", func(t *testing.T) { testInterleave(t, f32val) })
	t.Run("float64", func(t *testing.T) { testInterleave(t, f64val) })
	t.Run("complex64", func(t *testing.T) { testInterleave(t, c64val) })
	t.Run("complex128", func(t *testing.T) { testInterleave(t, c128val) })
}

func TestInterleaveRoundTrip(t *testing.T) {
	t.Run("float32", func(t *testing.T) { testRoundTrip(t, f32val) })
	t.Run("float64", func(t *testing.T) { testRoundTrip(t, f64val) })
	t.Run("complex64", func(t *testing.T) { testRoundTrip(t, c64val) })
	t.Run("complex128", func(t *testing.T) { testRoundTrip(t, c128val) })
}

// TestDeinterleaveShortest checks the frame count against every way the two
// sides can disagree, and that nothing past it is touched. The length logic is
// width-independent, so one width covers it.
func TestDeinterleaveShortest(t *testing.T) {
	const sentinel = -999.0
	cases := []struct {
		name   string
		src    int   // interleaved elements
		planes []int // plane lengths
		want   int   // frames expected
	}{
		{"exact", 12, []int{4, 4, 4}, 4},
		{"short src", 9, []int{4, 4, 4}, 3},
		{"partial frame", 11, []int{4, 4, 4}, 3},
		{"long src", 30, []int{4, 4, 4}, 4},
		{"one short plane", 12, []int{4, 2, 4}, 2},
		{"empty plane", 12, []int{4, 0, 4}, 0},
		{"no src", 0, []int{4, 4}, 0},
		{"no planes", 12, nil, 0},
		{"fewer elements than channels", 2, []int{4, 4, 4}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := make([]float64, tc.src)
			for i := range src {
				src[i] = float64(i)
			}
			planes := make([][]float64, len(tc.planes))
			for c, n := range tc.planes {
				planes[c] = make([]float64, n)
				for i := range planes[c] {
					planes[c][i] = sentinel
				}
			}
			n := Deinterleave(planes, src)
			if n != tc.want {
				t.Fatalf("wrote %d frames, want %d", n, tc.want)
			}
			for c, p := range planes {
				for i, v := range p {
					switch {
					case i < n:
						if want := src[i*len(planes)+c]; v != want {
							t.Errorf("channel %d frame %d = %v, want %v", c, i, v, want)
						}
					case v != sentinel:
						t.Errorf("channel %d frame %d was written past the %d frames reported", c, i, n)
					}
				}
			}
		})
	}
}

// TestInterleaveShortest is TestDeinterleaveShortest from the other side.
func TestInterleaveShortest(t *testing.T) {
	const sentinel = -999.0
	cases := []struct {
		name   string
		planes []int
		dst    int
		want   int
	}{
		{"exact", []int{4, 4, 4}, 12, 4},
		{"short dst", []int{4, 4, 4}, 9, 3},
		{"partial frame", []int{4, 4, 4}, 11, 3},
		{"long dst", []int{4, 4, 4}, 30, 4},
		{"one short plane", []int{4, 2, 4}, 12, 2},
		{"empty plane", []int{4, 0, 4}, 12, 0},
		{"no dst", []int{4, 4}, 0, 0},
		{"no planes", nil, 12, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			planes := make([][]float64, len(tc.planes))
			for c, n := range tc.planes {
				planes[c] = make([]float64, n)
				for i := range planes[c] {
					planes[c][i] = float64(c*100 + i)
				}
			}
			out := make([]float64, tc.dst)
			for i := range out {
				out[i] = sentinel
			}
			n := Interleave(out, planes)
			if n != tc.want {
				t.Fatalf("wrote %d frames, want %d", n, tc.want)
			}
			for i, v := range out {
				switch {
				case i < n*len(planes):
					if want := planes[i%len(planes)][i/len(planes)]; v != want {
						t.Errorf("element %d = %v, want %v", i, v, want)
					}
				case v != sentinel:
					t.Errorf("element %d was written past the %d frames reported", i, n)
				}
			}
		})
	}
}

// The benchmark arms, as package-level functions with identical signatures.
// Inlining and closure capture move this comparison by more than the loop order
// does -- enough to invert it -- so the arms have to be structured alike or the
// benchmark measures the harness.

func deinterleaveFrameOuter(dst [][]float64, src []float64) int {
	channels := len(dst)
	frames := len(src) / channels
	for _, p := range dst {
		frames = min(frames, len(p))
	}
	for i := range frames {
		for c := range channels {
			dst[c][i] = src[i*channels+c]
		}
	}
	return frames
}

// deinterleavePairs is the two-channel special case: both planes hoisted and
// the source read once.
func deinterleavePairs(dst [][]float64, src []float64) int {
	if len(dst) != 2 {
		return Deinterleave(dst, src)
	}
	frames := min(len(src)/2, len(dst[0]), len(dst[1]))
	l, r := dst[0][:frames], dst[1][:frames]
	for i := range frames {
		l[i] = src[i*2]
		r[i] = src[i*2+1]
	}
	return frames
}

// The two orders Interleave switches between, written out so the benchmark
// shows the crossover rather than only the branch taken.
func interleaveFrameOuter(dst []float64, src [][]float64) int {
	channels := len(src)
	frames := len(dst) / channels
	for _, p := range src {
		frames = min(frames, len(p))
	}
	if frames <= 0 {
		return 0
	}
	out := dst[:frames*channels]
	for i := range frames {
		f := out[i*channels:][:channels]
		for c, p := range src {
			f[c] = p[i]
		}
	}
	return frames
}

func interleaveChannelOuter(dst []float64, src [][]float64) int {
	channels := len(src)
	frames := len(dst) / channels
	for _, p := range src {
		frames = min(frames, len(p))
	}
	if frames <= 0 {
		return 0
	}
	out := dst[:frames*channels]
	for c, p := range src {
		p = p[:frames]
		dst := out[c:]
		for i, v := range p {
			dst[i*channels] = v
		}
	}
	return frames
}

func interleavePairs(dst []float64, src [][]float64) int {
	if len(src) != 2 {
		return Interleave(dst, src)
	}
	frames := min(len(dst)/2, len(src[0]), len(src[1]))
	l, r := src[0][:frames], src[1][:frames]
	for i := range frames {
		dst[i*2] = l[i]
		dst[i*2+1] = r[i]
	}
	return frames
}

// interleaveArena carves every buffer out of one allocation, skewing each
// region by 64 bytes: two same-sized make calls land at the same 4K offset,
// which costs about half the throughput to aliasing.
type interleaveArena struct {
	buf []float64
	off int
}

func newInterleaveArena(regions, each int) *interleaveArena {
	return &interleaveArena{buf: make([]float64, regions*(each+8))}
}

func (a *interleaveArena) take(n int) []float64 {
	s := a.buf[a.off : a.off+n : a.off+n]
	a.off += n + 8
	return s
}

// interleaveBenchBufs builds an interleaved buffer and its planes.
func interleaveBenchBufs(channels, frames int) ([]float64, [][]float64) {
	a := newInterleaveArena(channels+1, frames*channels)
	w := a.take(frames * channels)
	for i := range w {
		w[i] = float64(i) * 0.001
	}
	planes := make([][]float64, channels)
	for c := range planes {
		planes[c] = a.take(frames)
	}
	return w, planes
}

// BenchmarkDeinterleave is the loop-order question on its own: read the
// interleaved run once with a write stream per channel, or once per channel at
// a stride with one. The sizes sweep from L1-resident to memory-bound, since
// past the cache every arm converges on the bandwidth limit.
func BenchmarkDeinterleave(b *testing.B) {
	arms := []struct {
		name string
		fn   func([][]float64, []float64) int
	}{
		{"shipped", Deinterleave[float64]},
		{"frame-outer", deinterleaveFrameOuter},
		{"pairs", deinterleavePairs},
	}
	for _, channels := range []int{2, 4, 8} {
		for _, frames := range []int{256, 4096, 65536} {
			w, planes := interleaveBenchBufs(channels, frames)
			for _, arm := range arms {
				if arm.name == "pairs" && channels != 2 {
					continue
				}
				b.Run(fmt.Sprintf("%dch/%d/%s", channels, frames, arm.name), func(b *testing.B) {
					b.SetBytes(int64(len(w) * 8))
					for b.Loop() {
						arm.fn(planes, w)
					}
				})
			}
		}
	}
}

// BenchmarkInterleave is the same question from the other side, where the
// answer depends on the channel count. Both orders stay as arms so the
// crossover is visible and interleaveChannelOuterMax can be rechecked on a
// machine with a different cache.
func BenchmarkInterleave(b *testing.B) {
	arms := []struct {
		name string
		fn   func([]float64, [][]float64) int
	}{
		{"shipped", Interleave[float64]},
		{"channel-outer", interleaveChannelOuter},
		{"frame-outer", interleaveFrameOuter},
		{"pairs", interleavePairs},
	}
	for _, channels := range []int{2, 4, 8} {
		for _, frames := range []int{256, 4096, 65536} {
			w, planes := interleaveBenchBufs(channels, frames)
			for _, arm := range arms {
				if arm.name == "pairs" && channels != 2 {
					continue
				}
				b.Run(fmt.Sprintf("%dch/%d/%s", channels, frames, arm.name), func(b *testing.B) {
					b.SetBytes(int64(len(w) * 8))
					for b.Loop() {
						arm.fn(w, planes)
					}
				})
			}
		}
	}
}
