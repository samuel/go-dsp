package main

import (
	"log"
	"os"

	"github.com/samuel/go-sdr/rtl"
	"github.com/samuel/go-sdr/sdr"
)

const bufferSize = 32 * 1024 // in samples

func main() {
	dev, err := rtl.Open(0)
	if err != nil {
		log.Fatalf("Failed to open device: %s", err.Error())
	}
	defer dev.Close()

	sampleRate := 170000
	outputRate := 32000
	postDownsample := 4

	sampleRate *= 4
	frequency := int(92.7e6)
	downsample := 1000000/sampleRate + 1
	captureRate := downsample * sampleRate
	captureFreq := frequency + 16000 + captureRate/4

	// samples2 := rotate90(samples[:n/2])
	// samples2 = lowPass(samples2, 2)
	// samples2 = fmDemod(samples2)
	// samples2 = lowPassSimple(samples2, postDownSample)
	// samples2 = lowPassReal(samples2, sampleRate/postDownSample, outputRate)

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
	lowPass1 := &sdr.LowPassDownsampleComplexFilter{Downsample: 2}
	fmDemod := &sdr.FMDemodFilter{}
	lowPass2 := &sdr.LowPassDownsampleFilter{Downsample: postDownsample}
	lowPass3 := &sdr.LowPassDownsampleRationalFilter{Fast: sampleRate / postDownsample, Slow: outputRate}

	buf := make([]byte, bufferSize*2)
	samples := make([]complex64, bufferSize)
	pcm := make([]float32, bufferSize)
	for {
		n, err := dev.Read(buf)
		if err != nil {
			log.Fatal(err)
		}
		n /= 2
		for i := 0; i < n; i++ {
			samples[i] = complex(
				float32(buf[i*2])-128.0,
				float32(buf[i*2+1])-128.0,
			)
		}
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
		if pcm2, err = lowPass3.Filter(pcm2); err != nil {
			log.Fatal(err)
		}
		for i := 0; i < len(pcm2); i++ {
			v := int16(pcm2[i] * (1 << 14))
			buf[i*2] = uint8(uint16(v) & 0xff)
			buf[i*2+1] = uint8(uint16(v) >> 8)
		}
		if _, err := os.Stdout.Write(buf[:len(pcm2)*2]); err != nil {
			log.Fatal(err)
		}
	}
}
