// +build amd64

package dsp

import "testing"

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
}
