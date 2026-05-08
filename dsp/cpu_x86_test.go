//go:build 386 || amd64

package dsp

import (
	"testing"
)

func simdTest(t *testing.T, fn func(t *testing.T)) {
	if useSSE4 {
		t.Run("sse4", fn)
		useSSE4 = false
		t.Run("nosse4", fn)
		useSSE4 = true
	} else {
		t.Run("sse4", func(t *testing.T) { t.Skip("sse4 not available") })
		t.Run("nosse4", fn)
	}
	if useAVX2 {
		t.Run("avx2", fn)
		useAVX2 = false
		t.Run("noavx2", fn)
		useAVX2 = true
	} else {
		t.Run("avx2", func(t *testing.T) { t.Skip("avx2 not available") })
		t.Run("noavx2", fn)
	}
}
