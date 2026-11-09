//go:build !arm && !arm64

package simdcpu

// HaveNEON reports whether ARM NEON is available.
func HaveNEON() bool { return false }

// UseVector reports whether VFP vector ops should be used.
func UseVector() bool { return false }
