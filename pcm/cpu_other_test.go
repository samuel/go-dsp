//go:build !amd64 && !arm

package pcm

import "testing"

// simdTest runs fn once: nothing here dispatches at run time.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	t.Run("detected", fn)
}
