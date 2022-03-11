// +build arm

package dsp

import "testing"

func simdTest(t *testing.T, fn func(t *testing.T)) {
	if HaveNEON {
		t.Run("neon", fn)
		HaveNEON = false
		t.Run("noneon", fn)
		HaveNEON = true
	} else {
		t.Run("neon", func(t *testing.T) { t.Skip("NEON not available") })
		t.Run("noneon", fn)
	}
}
