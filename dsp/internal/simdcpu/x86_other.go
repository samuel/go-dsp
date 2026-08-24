//go:build !amd64

package simdcpu

// HasSSE4 reports whether SSE4.1 is available.
func HasSSE4() bool { return false }

// HasAVX reports whether AVX is available.
func HasAVX() bool { return false }
