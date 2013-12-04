package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"

	"github.com/samuel/go-sdr/sdr"
	"github.com/samuel/go-sdr/sdr/ax25"
)

var flagVerbose = flag.Bool("v", false, "Verbose output")

func main() {
	flag.Parse()

	rd := os.Stdin
	if len(flag.Args()) > 0 && flag.Arg(0) != "-" {
		fi, err := os.Open(flag.Arg(0))
		if err != nil {
			log.Fatal(err)
		}
		defer fi.Close()
		rd = fi
	}

	sampleRate := 44100
	baud := 1200
	window := 4
	interp := 1
	blockSize := sampleRate / baud

	goer := sdr.NewGoertzel([]int{1200, 2200}, sampleRate*interp, blockSize*interp)

	threshold := 50.0

	buf := make([]byte, window*2)
	samples := make([]float32, blockSize*interp)
	lastSample := float32(0.0)

	currentTime := float64(0.0)
	bitClock := 1.0 / float64(baud)
	windowTime := float64(window) / float64(sampleRate)
	timeDelta := 0.0
	prevBit := 0
	transition := false

	ax := ax25.NewDecoder()
	for {
		_, err := rd.Read(buf)
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		copy(samples, samples[window*interp:])

		si := len(samples) - window*interp
		for i := 0; i < len(buf); i += 2 {
			s := float32(int16(buf[i])|(int16(buf[i+1])<<8)) / 32768.0
			if interp > 1 {
				d := (s - lastSample) / float32(interp)
				for j := 1; j < interp; j++ {
					lastSample += d
					samples[si] = lastSample
					si++
				}
				lastSample = s
			}
			samples[si] = s
			si++
		}

		goer.Reset()
		goer.Feed(samples)
		mags := goer.Magnitude()
		diff := mags[0] - mags[1]

		if math.Abs(float64(diff)) > threshold {
			b := 1
			if diff < 0 {
				b = 0
			}
			if prevBit != b {
				transition = true
				prevBit = b
				// Align transitions to middle of clock tick
				timeDelta = bitClock/2.0 - currentTime
			}
		}

		currentTime += windowTime
		for currentTime >= bitClock {
			currentTime -= bitClock
			b := 1
			if transition {
				b = 0
				currentTime += timeDelta
				timeDelta = 0.0
			}
			frame := ax.Feed(b)
			if frame != nil {
				if *flagVerbose {
					fmt.Printf("%+v\n", frame)
				} else {
					fmt.Printf("%s to %s", frame.Source, frame.Destination)
					if len(frame.Repeaters) != 0 {
						fmt.Print(" via ")
						for i, r := range frame.Repeaters {
							if i != 0 {
								fmt.Print(",")
							}
							fmt.Print(r.String())
						}
					}
					fmt.Println()
				}
				fmt.Print(hex.Dump(frame.Info))
			}
			transition = false
		}
	}
}
