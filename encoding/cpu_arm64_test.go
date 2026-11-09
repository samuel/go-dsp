package encoding

import "testing"

func simdTest(t *testing.T, fn func(t *testing.T)) {
	if haveNEON {
		t.Run("neon", fn)
		haveNEON = false
		t.Run("noneon", fn)
		haveNEON = true
	} else {
		t.Run("neon", func(t *testing.T) { t.Skip("NEON not available") })
		t.Run("noneon", fn)
	}
}
