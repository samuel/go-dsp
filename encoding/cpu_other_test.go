//go:build !386 && !amd64 && !arm && !arm64

package encoding

import "testing"

// simdTest runs fn once. An architecture with no assembly here routes every
// entry point through asm_fallback.go to its Go reference, so there is nothing
// to toggle -- but the definition has to exist for the test binary to link,
// which is what gives the portable rung link coverage.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	t.Run("go", fn)
}
