package encoding

import "testing"

// simdTest runs fn once. The 386 assembly is all a JMP to the Go reference, so
// unlike the other architectures there are no feature flags to toggle.
func simdTest(t *testing.T, fn func(t *testing.T)) {
	t.Run("go", fn)
}
