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

const bufferSize = 256 * 1024 // in samples

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

	bufferCache := make(chan buffer, 32)
	sampleChan := make(chan buffer, 32)

	for i := 0; i < 32; i++ {
		bufferCache <- buffer{bytes: make([]byte, bufferSize*2)}
	}

	stopChan := make(chan bool)

	go func() {
		for {
			select {
			case _ = <-stopChan:
				return
			default:
			}

			buf := <-bufferCache
			n, err := dev.Read(buf.bytes)
			if err != nil {
				log.Fatal(err)
			}
			sampleChan <- buffer{bytes: buf.bytes, size: n}
		}
	}()

	go func() {
		bytes := make([]byte, bufferSize*2)
		samples := make([]complex64, bufferSize)
		pcm := make([]float32, bufferSize)
		for {
			select {
			case _ = <-stopChan:
				return
			default:
			}

			buf := <-sampleChan
			n := buf.size
			n /= 2
			for i := 0; i < n; i++ {
				samples[i] = complex(
					float32(buf.bytes[i*2])-128.0,
					float32(buf.bytes[i*2+1])-128.0,
				)
			}
			bufferCache <- buf

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
			for i := 0; i < len(pcm2); i++ {
				v := int16(pcm2[i] * (1 << 14))
				bytes[i*2] = uint8(uint16(v) & 0xff)
				bytes[i*2+1] = uint8(uint16(v) >> 8)
			}
			if _, err := os.Stdout.Write(bytes[:len(pcm2)*2]); err != nil {
				log.Fatal(err)
			}
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	_ = <-signalChan
	close(stopChan)

	if *flagCpuProfile {
		pprof.StopCPUProfile()
	}
}
