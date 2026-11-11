// Package simdcpu reports which vector instruction sets the running CPU
// supports.
//
// The kernels in internal/f32, internal/f64, and the sample codec in pcm
// dispatch on it, while the flag their assembly reads has to be a
// variable in the package that reads it — a Go assembly reference ·name(SB)
// resolves within its own package. Detection therefore lives here, and each
// consumer holds its own variable initialized from it, in a cpu.go nearly
// identical in both.
package simdcpu
