package sdr

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
)

var HaveNEON bool

var neonRE = regexp.MustCompile(`(?m)^Features.*neon.*$`)

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
}
