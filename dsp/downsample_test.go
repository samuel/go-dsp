package dsp

import "testing"

func TestBoxcarDecimator(t *testing.T) {
	src := []complex64{complex(0.0, 2.0), complex(1.0, 2.0), complex(-3.0, 7.0), complex(4.0, -9.0)}

	// A filter each, since the state carries across calls and there is no way
	// to rewind one.
	dst := make([]complex64, 256)
	copy(dst, src)
	f, err := NewBoxcarDecimator(2)
	if err != nil {
		t.Fatal(err)
	}
	dst = dst[:boxcarDecimateAsm(f, dst, dst)]

	expected := make([]complex64, 256)
	copy(expected, src)
	f, err = NewBoxcarDecimator(2)
	if err != nil {
		t.Fatal(err)
	}
	expected = expected[:boxcarDecimate(f, expected, expected)]

	if len(dst) != len(expected) {
		t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
	}
	for i := range dst {
		if dst[i] != expected[i] {
			t.Fatalf("Output doesn't match expected:\n%+v\n%+v", dst, expected)
		}
	}
}

func TestRationalBoxcarDecimator(t *testing.T) {
	src := make([]float32, 256)
	for i := range src {
		src[i] = float32(i - 128)
	}

	dst := make([]float32, 256)
	copy(dst, src)
	f, err := NewRationalBoxcarDecimator(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	dst = dst[:rationalBoxcarDecimateAsm(f, dst, dst)]

	expected := make([]float32, 256)
	copy(expected, src)
	f, err = NewRationalBoxcarDecimator(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	expected = expected[:rationalBoxcarDecimate(f, expected, expected)]

	if len(dst) != len(expected) {
		t.Fatalf("Output doesn't match expected: %+v != %+v", dst, expected)
	}
	for i := range dst {
		if dst[i] != expected[i] {
			t.Fatalf("Output doesn't match expected: %+v != %+v", dst, expected)
		}
	}
}

// TestRationalBoxcarDecimatorRipple checks that a non-integer ratio has
// no periodic gain ripple. Each output averages either two input samples or
// one, so scaling both by a fixed Slow/Fast used to put an alternating gain --
// a tone at the beat frequency -- on top of the signal. On a ramp the output
// has to be a ramp.
func TestRationalBoxcarDecimatorRipple(t *testing.T) {
	src := make([]float32, 300)
	for i := range src {
		src[i] = float32(i)
	}
	f, err := NewRationalBoxcarDecimator(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	out := f.FilterInPlace(src)
	if len(out) < 100 {
		t.Fatalf("expected around 200 outputs, got %d", len(out))
	}
	step := out[2] - out[1]
	for i := 2; i < len(out)-1; i++ {
		if d := out[i+1] - out[i]; d != step {
			t.Fatalf("dst %d steps by %v, not the uniform %v: %v", i, d, step, out[:10])
		}
	}
}

func BenchmarkBoxcarDecimator(b *testing.B) {
	filter, err := NewBoxcarDecimator(2)
	if err != nil {
		b.Fatal(err)
	}
	src := make([]complex64, 256)
	for i := range 256 {
		src[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for range b.N {
		_ = boxcarDecimateAsm(filter, src, src)
	}
}

func BenchmarkBoxcarDecimator_Go(b *testing.B) {
	filter, err := NewBoxcarDecimator(2)
	if err != nil {
		b.Fatal(err)
	}
	src := make([]complex64, 256)
	for i := range 256 {
		src[i] = complex(float32(i)-128.0, -(float32(i) - 128.0))
	}
	for range b.N {
		_ = boxcarDecimate(filter, src, src)
	}
}

func BenchmarkRationalBoxcarDecimator(b *testing.B) {
	filter, err := NewRationalBoxcarDecimator(3, 2)
	if err != nil {
		b.Fatal(err)
	}
	src := make([]float32, 256)
	for i := range 256 {
		src[i] = float32(i) - 128.0
	}
	for range b.N {
		_ = rationalBoxcarDecimateAsm(filter, src, src)
	}
}

func BenchmarkRationalBoxcarDecimator_Go(b *testing.B) {
	filter, err := NewRationalBoxcarDecimator(3, 2)
	if err != nil {
		b.Fatal(err)
	}
	src := make([]float32, 256)
	for i := range 256 {
		src[i] = float32(i) - 128.0
	}
	for range b.N {
		_ = rationalBoxcarDecimate(filter, src, src)
	}
}

// TestDownsampleReset checks that a reset decimator reproduces its first output,
// which is the only way to reuse one on a new stream.
func TestDownsampleReset(t *testing.T) {
	src := make([]float32, 64)
	for i := range src {
		src[i] = float32(i)
	}

	f, err := NewRationalBoxcarDecimator(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	first := append([]float32(nil), src...)
	first = f.FilterInPlace(first)
	f.Reset()
	again := append([]float32(nil), src...)
	again = f.FilterInPlace(again)
	if len(again) != len(first) {
		t.Fatalf("after Reset got %d outputs, want %d", len(again), len(first))
	}
	for i := range first {
		if again[i] != first[i] {
			t.Fatalf("dst %d after Reset: %v, want %v", i, again[i], first[i])
		}
	}

	c, err := NewBoxcarDecimator(3)
	if err != nil {
		t.Fatal(err)
	}
	cin := make([]complex64, 64)
	for i := range cin {
		cin[i] = complex(float32(i), float32(-i))
	}
	cFirst := c.FilterInPlace(append([]complex64(nil), cin...))
	want := append([]complex64(nil), cFirst...)
	c.Reset()
	cAgain := c.FilterInPlace(append([]complex64(nil), cin...))
	if len(cAgain) != len(want) {
		t.Fatalf("after Reset got %d outputs, want %d", len(cAgain), len(want))
	}
	for i := range want {
		if cAgain[i] != want[i] {
			t.Fatalf("complex dst %d after Reset: %v, want %v", i, cAgain[i], want[i])
		}
	}
}

func TestNewDownsampleRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"complex zero", func() error { _, err := NewBoxcarDecimator(0); return err }},
		{"complex negative", func() error { _, err := NewBoxcarDecimator(-2); return err }},
		{"rational zero fast", func() error { _, err := NewRationalBoxcarDecimator(0, 8); return err }},
		{"rational zero", func() error { _, err := NewRationalBoxcarDecimator(8, 0); return err }},
		{"rational negative", func() error { _, err := NewRationalBoxcarDecimator(-8, -2); return err }},
		{"rational upsample", func() error { _, err := NewRationalBoxcarDecimator(2, 3); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// TestDecimatorFilterMatchesInPlace pins the two forms against each other,
// including across block boundaries that leave a partial group carried. Both
// go through the same kernel, one with dst aliasing src, so this covers the
// aliased and separate destinations against each other.
func TestDecimatorFilterMatchesInPlace(t *testing.T) {
	src := make([]complex64, 501)
	for i := range src {
		src[i] = complex(float32(i)*0.5-3, float32(i)*-0.25+1)
	}
	for _, factor := range []int{1, 2, 3, 7, 16} {
		for _, block := range []int{len(src), 1, 3, factor, factor + 1, 64} {
			a, _ := NewBoxcarDecimator(factor)
			b, _ := NewBoxcarDecimator(factor)
			var want, got []complex64
			for i := 0; i < len(src); i += block {
				in := src[i:min(i+block, len(src))]
				want = append(want, a.FilterInPlace(append([]complex64(nil), in...))...)

				out := make([]complex64, b.OutputLen(len(in))+1)
				n := b.Filter(out, in)
				if n != len(out)-1 {
					t.Fatalf("factor=%d block=%d: wrote %d, OutputLen said %d", factor, block, n, len(out)-1)
				}
				if out[n] != 0 {
					t.Fatalf("factor=%d block=%d: wrote past the end", factor, block)
				}
				got = append(got, out[:n]...)
			}
			if len(got) != len(want) {
				t.Fatalf("factor=%d block=%d: %d outputs, want %d", factor, block, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("factor=%d block=%d [%d] = %v, want %v", factor, block, i, got[i], want[i])
				}
			}
		}
	}
}

func TestRationalDecimatorFilterMatchesInPlace(t *testing.T) {
	src := make([]float32, 499)
	for i := range src {
		src[i] = float32(i%23) - 11
	}
	for _, r := range [][2]int{{48000, 48000}, {48000, 44100}, {48000, 16000}, {3, 2}} {
		for _, block := range []int{len(src), 1, 5, 64} {
			a, _ := NewRationalBoxcarDecimator(r[0], r[1])
			b, _ := NewRationalBoxcarDecimator(r[0], r[1])
			var want, got []float32
			for i := 0; i < len(src); i += block {
				in := src[i:min(i+block, len(src))]
				want = append(want, a.FilterInPlace(append([]float32(nil), in...))...)

				out := make([]float32, b.OutputLen(len(in))+1)
				n := b.Filter(out, in)
				if n != len(out)-1 {
					t.Fatalf("%v block=%d: wrote %d, OutputLen said %d", r, block, n, len(out)-1)
				}
				got = append(got, out[:n]...)
			}
			if len(got) != len(want) {
				t.Fatalf("%v block=%d: %d outputs, want %d", r, block, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%v block=%d [%d] = %v, want %v", r, block, i, got[i], want[i])
				}
			}
		}
	}
}

// Filter with dst aliasing src is the in-place form, and has to agree with it.
func TestDecimatorFilterInPlaceAliasing(t *testing.T) {
	src := make([]complex64, 64)
	for i := range src {
		src[i] = complex(float32(i), 0)
	}
	a, _ := NewBoxcarDecimator(4)
	want := a.FilterInPlace(append([]complex64(nil), src...))
	b, _ := NewBoxcarDecimator(4)
	got := append([]complex64(nil), src...)
	n := b.Filter(got, got)
	if n != len(want) {
		t.Fatalf("wrote %d, want %d", n, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestRationalOutputLenDoesNotOverflow checks the count for an input long
// enough that inputs*slow leaves the range of a 32-bit int.
//
// It can only fail on a 32-bit target -- 386 and arm here -- where the old
// int arithmetic wrapped to a negative number, OutputLen returned a short
// answer, and a caller sizing dst from it watched Filter run off the end. Run
// it with qemu-test -arch arm ./dsp.
func TestRationalOutputLenDoesNotOverflow(t *testing.T) {
	f, err := NewRationalBoxcarDecimator(44100, 8000)
	if err != nil {
		t.Fatal(err)
	}
	// 8000 * 300000 is 2.4e9, past math.MaxInt32.
	const inputs = 300000
	want := int(int64(inputs) * 8000 / 44100)
	if got := f.OutputLen(inputs); got != want {
		t.Fatalf("OutputLen(%d) = %d, want %d", inputs, got, want)
	}

	// And the count is what Filter actually writes.
	src := make([]float32, inputs)
	dst := make([]float32, want+1)
	dst[want] = -12345.5
	if n := f.Filter(dst, src); n != want {
		t.Fatalf("Filter wrote %d samples, OutputLen said %d", n, want)
	}
	if dst[want] != -12345.5 {
		t.Fatal("Filter wrote past the length OutputLen reported")
	}
}

// TestDecimatorRatesAreWhatWasAsked checks the accessors the measurement layer
// reads to label a frequency axis. Nothing else reads them, so a wrong one
// would show up only as a plot with the wrong numbers on it.
func TestDecimatorRatesAreWhatWasAsked(t *testing.T) {
	d, err := NewBoxcarDecimator(5)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Factor(); got != 5 {
		t.Errorf("Factor = %d, want 5", got)
	}
	// The factor is fixed at construction, so filtering must not move it.
	d.Filter(make([]complex64, 4), make([]complex64, 17))
	if got := d.Factor(); got != 5 {
		t.Errorf("Factor after Filter = %d, want 5", got)
	}

	r, err := NewRationalBoxcarDecimator(48, 11)
	if err != nil {
		t.Fatal(err)
	}
	if fast, slow := r.Rates(); fast != 48 || slow != 11 {
		t.Errorf("Rates = (%d, %d), want (48, 11)", fast, slow)
	}
	r.Filter(make([]float32, r.OutputLen(100)), make([]float32, 100))
	if fast, slow := r.Rates(); fast != 48 || slow != 11 {
		t.Errorf("Rates after Filter = (%d, %d), want (48, 11)", fast, slow)
	}
}
