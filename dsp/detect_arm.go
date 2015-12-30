package dsp

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
)

var (
	// HaveNEON is true if ARM NEON SIMD instructions are available
	HaveNEON bool
	// UseVector is true if VFP vector ops should be used
	UseVector bool
)

var (
	// neonRE matches /proc/cpuinfo if the neon instruction set is available
	neonRE = regexp.MustCompile(`(?m)^Features.*neon.*$`)
	// rpi1RE matches /proc/cpuinfo for Raspberry Pi 1
	rpi1RE = regexp.MustCompile(`(?m)^Hardware.*BCM2708.*$`)
)

func init() {
	// ARM doesn't expose CPU info to userland so it's necessary to
	// get the information from the kernel.
	// Ref: Cortex-A Series Programmer's Guide Section 20.1.7 Detecting NEON

	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}
	defer f.Close()

	b, err := ioutil.ReadAll(io.LimitReader(f, 2048))
	if err != nil {
		log.Printf("Failed to read cpuinfo: %s", err.Error())
		return
	}

	HaveNEON = neonRE.Match(b)
	// Vector ops are considerably slower on more recent ARM (ARM8, ARM9).
	// These generally have NEON anyway. Only enable vfp vector use for
	// Raspberry Pi 1 to be safe.
	UseVector = !HaveNEON && rpi1RE.Match(b)
	if b, err := strconv.ParseBool(os.Getenv("ARMVECTOR")); err == nil {
		UseVector = b
	}
}
