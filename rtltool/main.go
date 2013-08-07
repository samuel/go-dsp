package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"

	"github.com/samuel/go-sdr/rtl"
	"github.com/samuel/go-sdr/sdr"
)

const (
	nBuffers   = 32
	bufferSize = 256 * 1024 // in samples
)

var flagCpuProfile = flag.Bool("profile.cpu", false, "Enable CPU profiling")

type buffer struct {
	bytes []byte
	size  int
}

func main() {
	flag.Parse()

	if *flagCpuProfile {
		wr, err := os.Create("cpu.prof")
		if err != nil {
			log.Fatal(err)
		}
		defer wr.Close()

		if err := pprof.StartCPUProfile(wr); err != nil {
			log.Fatal(err)
		}
	}

	dev, err := rtl.Open(0)
	if err != nil {
		log.Fatalf("Failed to open device: %s", err.Error())
	}
	defer dev.Close()

	sampleRate := 170000
	outputRate := 32000
	postDownsample := 4

	sampleRate *= postDownsample
	frequency := int(92.7e6)
	downsample := 1000000/sampleRate + 1
	captureRate := downsample * sampleRate
	captureFreq := frequency + 16000 + captureRate/4

	fmt.Fprintf(os.Stderr, "Oversampling input by: %dx\n", downsample)
	fmt.Fprintf(os.Stderr, "Oversampling output by: %dx\n", postDownsample)
	fmt.Fprintf(os.Stderr, "Sampling at %d Hz\n", captureRate)
	fmt.Fprintf(os.Stderr, "Tuned to %d Hz\n", captureFreq)

	if err := dev.SetSampleRate(uint(captureRate)); err != nil {
		log.Fatalf("Failed to set sample rate: %s", err.Error())
	}

	if err := dev.SetCenterFreq(uint(captureFreq)); err != nil {
		log.Fatalf("Failed to set center freq: %s", err.Error())
	}

	if err := dev.ResetBuffer(); err != nil {
		log.Fatalf("Failed to reset buffers: %s", err.Error())
	}

	rotate90 := &sdr.Rotate90Filter{}
	lowPass1 := &sdr.LowPassDownsampleComplexFilter{Downsample: downsample}
	fmDemod := &sdr.FMDemodFilter{}
	// lowPass2 := &sdr.LowPassDownsampleRationalFilter{Fast: postDownsample, Slow: 1}
	// lowPass3 := &sdr.LowPassDownsampleRationalFilter{Fast: sampleRate / postDownsample, Slow: outputRate}
	lowPass2 := &sdr.LowPassDownsampleRationalFilter{Fast: sampleRate, Slow: outputRate}

	stopChan := make(chan bool)

	bytes := make([]byte, bufferSize*2)
	samples := make([]complex64, bufferSize)
	pcm := make([]float32, bufferSize)
	dev.ReadAsync(nBuffers, bufferSize, func(buf []byte) bool {
		for {
			select {
			case _ = <-stopChan:
				return true
			default:
			}

			n := len(buf)
			n /= 2
			sdr.Ui8toc64(buf, samples[:n])

			var samples2 []complex64
			if samples2, err = rotate90.Filter(samples[:n]); err != nil {
				log.Fatal(err)
			}
			if samples2, err = lowPass1.Filter(samples2); err != nil {
				log.Fatal(err)
			}
			n, err = fmDemod.Demodulate(samples2, pcm)
			if err != nil {
				log.Fatal(err)
			}
			var pcm2 []float32
			if pcm2, err = lowPass2.Filter(pcm[:n]); err != nil {
				log.Fatal(err)
			}
			// if pcm2, err = lowPass3.Filter(pcm2); err != nil {
			// 	log.Fatal(err)
			// }
			sdr.F32toi16b(pcm2, bytes, 1<<14)
			if _, err := os.Stdout.Write(bytes[:len(pcm2)*2]); err != nil {
				log.Fatal(err)
			}

			return false
		}
	})

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	_ = <-signalChan
	close(stopChan)

	if *flagCpuProfile {
		pprof.StopCPUProfile()
	}
}
