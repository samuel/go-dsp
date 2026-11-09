package simdcpu

import (
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
)

var (
	haveNEON  bool
	useVector bool
)

var (
	// neonRE matches /proc/cpuinfo if the neon instruction set is available
	neonRE = regexp.MustCompile(`(?m)^Features.*(neon|asimd).*$`)
	rpi1RE = regexp.MustCompile(`(?m)^Hardware.*BCM2708.*$`)
)

func init() {
	// ARM does not expose CPU info to userland, so this parses /proc/cpuinfo.
	// Ref: Cortex-A Series Programmer's Guide 20.1.7, Detecting NEON.
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck // opened read-only, so there is nothing a close error could tell us

	b, err := io.ReadAll(io.LimitReader(f, 2048))
	if err != nil {
		slog.Warn("dsp: failed to read /proc/cpuinfo, assuming no NEON", "error", err)
		return
	}

	haveNEON = neonRE.Match(b)
	// Vector ops are considerably slower on newer ARM (ARM8, ARM9), which have
	// NEON anyway; enable VFP vector use only for Raspberry Pi 1, to be safe.
	useVector = !haveNEON && rpi1RE.Match(b)
	if b, err := strconv.ParseBool(os.Getenv("ARMVECTOR")); err == nil {
		useVector = b
	}
}

// HaveNEON reports whether ARM NEON is available.
func HaveNEON() bool { return haveNEON }

// UseVector reports whether VFP vector ops should be used.
func UseVector() bool { return useVector }
