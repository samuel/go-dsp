package simdcpu

// HaveNEON reports whether ARM NEON is available. NEON is part of the arm64
// baseline, so it needs no runtime detection.
func HaveNEON() bool { return true }

// UseVector reports whether VFP vector ops should be used. They exist only on
// 32-bit arm.
func UseVector() bool { return false }
