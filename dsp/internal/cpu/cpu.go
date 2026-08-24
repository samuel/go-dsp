// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build amd64

// Package cpu reports the x86 features the dispatchers in dsp/internal/simdcpu
// select on.
//
// It is a trimmed copy of the Go runtime's internal/cpu: it detects only the
// features this library needs, and builds only for amd64, since no
// other architecture imports it.
//
// One behavior differs from upstream: a GODEBUG override applies at every
// GOAMD64 level. The runtime refuses to turn off a feature its baseline
// requires because its own compiled code assumes it; here the flags only
// choose between kernels that are all correct, so GODEBUG=cpu.avx=off usefully
// forces the SSE leaf even in a GOAMD64=v3 build.
package cpu

// cacheLinePadSize is the assumed cache line size, used only to pad X86 below.
const cacheLinePadSize = 64

// The booleans in X86 contain the correspondingly named cpuid feature bit.
// HasAVX is only set if the OS supports XMM and YMM registers in addition to
// the cpuid feature bit being set. The struct is written once during
// Initialize and read on every dispatch after that, so it is padded to keep a
// neighboring global from sharing its cache line.
var X86 struct {
	_        [cacheLinePadSize]byte
	HasAVX   bool
	HasSSE41 bool
	_        [cacheLinePadSize]byte
}

// cpuid is implemented in cpu_amd64.s.
func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)

// xgetbv with ecx = 0 is implemented in cpu_amd64.s.
func xgetbv() (eax, edx uint32)

// ecx bits of cpuid leaf 1.
const (
	cpuidSSE41   = 1 << 19
	cpuidOSXSAVE = 1 << 27
	cpuidAVX     = 1 << 28
)

// Initialize examines the processor and sets the variables above. env is a
// GODEBUG-style string, which may override what was detected.
func Initialize(env string) {
	doinit()
	processOptions(env)
}

func doinit() {
	options = []option{
		{Name: "avx", Feature: &X86.HasAVX},
		{Name: "sse41", Feature: &X86.HasSSE41},
	}

	maxID, _, _, _ := cpuid(0, 0)
	if maxID < 1 {
		return
	}

	_, _, ecx1, _ := cpuid(1, 0)
	X86.HasSSE41 = isSet(ecx1, cpuidSSE41)

	// OSXSAVE can be false when using older operating systems, or when
	// explicitly disabled on newer ones by e.g. setting the xsavedisable boot
	// option on Windows 10. For XGETBV it is required and sufficient.
	var osSupportsAVX bool
	if isSet(ecx1, cpuidOSXSAVE) {
		eax, _ := xgetbv()
		// Check if XMM and YMM registers have OS support.
		osSupportsAVX = isSet(eax, 1<<1) && isSet(eax, 1<<2)
	}
	X86.HasAVX = isSet(ecx1, cpuidAVX) && osSupportsAVX
}

func isSet(hwc uint32, value uint32) bool {
	return hwc&value != 0
}

// options contains the cpu debug options that can be used in GODEBUG.
var options []option

// Option names should be lower case. e.g. avx instead of AVX.
type option struct {
	Name      string
	Feature   *bool
	Specified bool // whether feature value was specified in GODEBUG
	Enable    bool // whether feature should be enabled
}

// processOptions enables or disables CPU feature values based on the parsed env string.
// The env string is expected to be of the form cpu.feature1=value1,cpu.feature2=value2...
// where feature names is one of the list stored in the options variable and values are
// either 'on' or 'off'. If env contains cpu.all=off then all cpu features referenced
// through the options variable are disabled. Other feature names and values result in
// warning messages.
func processOptions(env string) {
field:
	for env != "" {
		var field string
		i := indexByte(env, ',')
		if i < 0 {
			field, env = env, ""
		} else {
			field, env = env[:i], env[i+1:]
		}
		if len(field) < 4 || field[:4] != "cpu." {
			continue
		}
		i = indexByte(field, '=')
		if i < 0 {
			print("GODEBUG: no value specified for \"", field, "\"\n")
			continue
		}
		key, value := field[4:i], field[i+1:] // e.g. "avx", "on"

		var enable bool
		switch value {
		case "on":
			enable = true
		case "off":
			enable = false
		default:
			print("GODEBUG: value \"", value, "\" not supported for cpu option \"", key, "\"\n")
			continue field
		}

		if key == "all" {
			for i := range options {
				options[i].Specified = true
				options[i].Enable = enable
			}
			continue field
		}

		for i := range options {
			if options[i].Name == key {
				options[i].Specified = true
				options[i].Enable = enable
				continue field
			}
		}

		// Not every GODEBUG cpu option names a feature this package detects,
		// so an unknown one is not worth a warning.
	}

	for _, o := range options {
		if !o.Specified {
			continue
		}

		if o.Enable && !*o.Feature {
			print("GODEBUG: can not enable \"", o.Name, "\", missing CPU support\n")
			continue
		}

		*o.Feature = o.Enable
	}
}

// indexByte returns the index of the first instance of c in s,
// or -1 if c is not present in s.
// indexByte is semantically the same as [strings.IndexByte].
// We copy this function because "internal/cpu" should not have external dependencies.
func indexByte(s string, c byte) int {
	for i := range len(s) {
		if s[i] == c {
			return i
		}
	}
	return -1
}
