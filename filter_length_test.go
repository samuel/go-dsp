package dsp

import (
	"math"
	"slices"
	"testing"
)

// TestFilterShortDestination checks the four 1:1 filters against the rule the
// V family already follows: process min(len(dst), len(src)) samples, and
// consume no more input than that.
//
// Both halves matter. Writing len(src) outputs into a shorter dst is a panic at
// best and a corrupted neighbor at worst, and the four used to document
// "dst must be at least as long as src" rather than enforce it. Consuming the
// whole of src while writing only part of it is subtler: the delay lines would
// then hold samples the caller never saw come out, so the next block would
// start from a state that does not match the stream.
func TestFilterShortDestination(t *testing.T) {
	src := rampAndTone(32)

	// Each entry filters src into dst using a freshly built filter, so a case
	// can be run twice and compared.
	filters := []struct {
		name string
		run  func(t *testing.T, dst, src []float64)
	}{
		{"IIRFilter", func(t *testing.T, dst, src []float64) {
			f, err := NewIIRFilter([]float64{0.2, 0.3, 0.2}, []float64{1, -0.4, 0.1})
			if err != nil {
				t.Fatal(err)
			}
			f.Filter(dst, src)
		}},
		{"BiQuadFilter", func(t *testing.T, dst, src []float64) {
			f, err := NewLowPassBiQuad[float64](48000, 4000, 0.7071)
			if err != nil {
				t.Fatal(err)
			}
			f.Filter(dst, src)
		}},
		{"DCFilter", func(t *testing.T, dst, src []float64) {
			f, err := NewDCFilter[float64](0.95)
			if err != nil {
				t.Fatal(err)
			}
			f.Filter(dst, src)
		}},
		{"FIRFilter", func(t *testing.T, dst, src []float64) {
			f, err := NewFIRFilter([]float64{0.1, 0.2, 0.4, 0.2, 0.1})
			if err != nil {
				t.Fatal(err)
			}
			f.Filter(dst, src)
		}},
	}

	for _, fl := range filters {
		t.Run(fl.name, func(t *testing.T) {
			full := make([]float64, len(src))
			fl.run(t, full, src)

			for n := range len(src) + 1 {
				// One extra element, left at a value no filter here can
				// produce, to catch a write past the end of dst.
				const guard = -12345.5
				dst := make([]float64, n+1)
				dst[n] = guard
				fl.run(t, dst[:n], src)

				if dst[n] != guard {
					t.Fatalf("dst of %d: wrote past the end", n)
				}
				if !slices.Equal(dst[:n], full[:n]) {
					t.Fatalf("dst of %d: got %v, want the first %d of %v", n, dst[:n], n, full)
				}
			}
		})
	}
}

// TestFilterShortDestinationDoesNotConsumeMore checks the state half of the
// rule: filtering a short block and then the rest has to give the same stream
// as filtering the whole thing, which only holds if a short dst leaves the
// unwritten input unconsumed.
func TestFilterShortDestinationDoesNotConsumeMore(t *testing.T) {
	src := rampAndTone(32)
	const split = 12

	for _, fl := range []struct {
		name  string
		build func() func(dst, src []float64)
	}{
		{"IIRFilter", func() func(dst, src []float64) {
			f, _ := NewIIRFilter([]float64{0.2, 0.3, 0.2}, []float64{1, -0.4, 0.1})
			return f.Filter
		}},
		{"BiQuadFilter", func() func(dst, src []float64) {
			f, _ := NewLowPassBiQuad[float64](48000, 4000, 0.7071)
			return f.Filter
		}},
		{"DCFilter", func() func(dst, src []float64) {
			f, _ := NewDCFilter[float64](0.95)
			return f.Filter
		}},
		{"FIRFilter", func() func(dst, src []float64) {
			f, _ := NewFIRFilter([]float64{0.1, 0.2, 0.4, 0.2, 0.1})
			return f.Filter
		}},
	} {
		t.Run(fl.name, func(t *testing.T) {
			want := make([]float64, len(src))
			fl.build()(want, src)

			got := make([]float64, len(src))
			f := fl.build()
			// A dst shorter than src for the first call, then the rest.
			f(got[:split], src)
			f(got[split:], src[split:])
			if !slices.Equal(got, want) {
				t.Fatalf("split at %d: %v, want %v", split, got, want)
			}
		})
	}
}

// TestBiQuadFilterOneMatchesFilter requires the two entry points to agree bit
// for bit, not merely closely.
//
// Filter divides the five coefficients by A0 once for the whole block while
// FilterOne used to divide the accumulated sum by A0 instead. Those are
// different roundings, so the two drifted apart -- and because the delay lines
// feed back, the drift compounds rather than staying at one ulp. They share
// norm and step now, which is what makes an exact comparison the right one.
func TestBiQuadFilterOneMatchesFilter(t *testing.T) {
	src := rampAndTone(256)

	for _, tc := range []struct {
		name  string
		build func() (*BiQuadFilter[float64], error)
	}{
		{"lowpass", func() (*BiQuadFilter[float64], error) {
			return NewLowPassBiQuad[float64](48000, 1000, 0.7071)
		}},
		{"peakingeq", func() (*BiQuadFilter[float64], error) {
			return NewPeakingEQBiQuad[float64](48000, 3000, 4, 12)
		}},
		// A0 far from 1, so dividing the sum and dividing the coefficients
		// round differently enough to see.
		{"unnormalized", func() (*BiQuadFilter[float64], error) {
			return NewBiQuad[float64](0.3, 0.7, 0.11, 3.7, -1.3, 0.29)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block, err := tc.build()
			if err != nil {
				t.Fatal(err)
			}
			single, err := tc.build()
			if err != nil {
				t.Fatal(err)
			}

			dst := make([]float64, len(src))
			block.Filter(dst, src)
			for i, s := range src {
				if got := single.FilterOne(s); got != dst[i] {
					t.Fatalf("sample %d: FilterOne %v, Filter %v (differ by %g)",
						i, got, dst[i], math.Abs(got-dst[i]))
				}
			}
		})
	}
}
