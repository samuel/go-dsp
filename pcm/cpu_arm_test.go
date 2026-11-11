package pcm

import "testing"

// simdTest runs fn with NEON as detected and then with it off, which takes
// the VFP paths.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	t.Run("detected", fn)
	if haveNEON {
		haveNEON = false
		defer func() { haveNEON = true }()
		t.Run("noneon", fn)
	}
}
