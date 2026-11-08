//go:build !386 && !amd64 && !arm && !arm64

package f32

import "testing"

// simdTest runs fn once. An architecture with no assembly here uses the scalar
// implementations directly, so there is nothing to toggle -- but the definition
// has to exist for the test binary to link, which is what gives the portable
// rung link coverage.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	t.Run("go", fn)
}
