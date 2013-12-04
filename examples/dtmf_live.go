package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"code.google.com/p/portaudio-go/portaudio"
	"github.com/samuel/go-sdr/sdr/dtmf"
)

func main() {
	sampleRate := 44100
	blockSize := 205 * sampleRate / 8000
	window := blockSize / 4
	dt := dtmf.NewStandard(sampleRate, blockSize)
	lastKey := -1
	keyCount := 0
	samples := make([]float32, blockSize)

	if err := portaudio.Initialize(); err != nil {
		log.Fatalf("Initialize: %+v", err)
	}
	defer func() {
		if err := portaudio.Terminate(); err != nil {
			log.Fatalf("Terminate: %+v", err)
		}
	}()
	inputBuf := make([]float32, window)
	stream, err := portaudio.OpenDefaultStream(1, 0, float64(sampleRate), len(inputBuf), inputBuf)
	if err != nil {
		log.Fatalf("OpenDefaultStream: %+v", err)
	}
	defer stream.Close()
	if err := stream.Start(); err != nil {
		log.Fatalf("Start: %+v", err)
	}
	defer stream.Stop()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)
	for {
		if err := stream.Read(); err != nil {
			log.Fatalf("Read: %+v", err)
		}

		copy(samples, samples[window:])
		copy(samples[len(samples)-len(inputBuf):], inputBuf)

		if k, t := dt.Feed(samples); k == lastKey && t > 0.0 {
			keyCount++
			if keyCount == 10 {
				fmt.Printf("%c", dtmf.Keypad[k])
			}
		} else {
			lastKey = k
			keyCount = 0
		}

		select {
		case <-sig:
			fmt.Println()
			return
		default:
		}
	}
}
