package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/samuel/go-dsp/dsp/dtmf"
)

func main() {
	sampleRate := 8000
	blockSize := 205 * sampleRate / 8000
	window := blockSize / 4
	dt := dtmf.NewStandard(sampleRate, blockSize)
	lastKey := -1
	keyCount := 0
	samples := make([]float32, blockSize)

	rd := os.Stdin
	if len(os.Args) > 1 && os.Args[1] != "-" {
		fi, err := os.Open(os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		defer fi.Close()
		rd = fi
	}

	buf := make([]byte, window*2)

	for {
		_, err := rd.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		copy(samples, samples[window:])

		si := len(samples) - window
		for i := 0; i < len(buf); i += 2 {
			s := float32(int16(buf[i])|(int16(buf[i+1])<<8)) / 32768.0
			samples[si] = s
			si++
		}

		if k, t := dt.Feed(samples); k == lastKey && t > 0.0 {
			keyCount++
			if keyCount == 9 {
				fmt.Printf("%c", dtmf.Keypad[k])
			}
		} else {
			lastKey = k
			keyCount = 0
		}
	}
	fmt.Println()
}
