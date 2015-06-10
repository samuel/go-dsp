package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"time"

	"github.com/samuel/go-dsp/borip"
)

func polarDiscriminant(a, b complex128) float64 {
	c := a * complex(real(b), -imag(b))
	angle := math.Atan2(imag(c), real(c))
	return angle / math.Pi
}

func main() {
	bor, err := borip.Dial("127.0.0.1:28888")
	if err != nil {
		log.Fatal(err)
	}
	defer bor.Close()

	dev, err := bor.SelectDevice("RTL")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "%+v\n", dev)

	freq := 484.7e6
	// freq := 162.4e6
	// freq := 92.7e6

	targetIF, actualIF, _, _, err := bor.SetFrequency(freq)
	if err != nil {
		log.Fatal(err)
	}
	queryFreq, err := bor.Frequency()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Freq: %f %f (%f)\n", targetIF, actualIF, queryFreq)
	if err := bor.SetAntenna(dev.ValidAntennas[0]); err != nil {
		log.Fatal(err)
	}
	// ant, err := bor.Antenna()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("Antenna: %s\n", ant)

	rate := 1.0e6
	actualRate, err := bor.SetRate(rate)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Actual rate: %f\n", actualRate)
	// actualRate, err = bor.Rate()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("Actual rate: %f\n", actualRate)

	// err = bor.SetGain(-1.0)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// gain, err := bor.Gain()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("Gain: %f\n", gain)
	// dest, err := bor.Destination()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Printf("Destination: %s\n", dest)
	// if err := bor.SetHeaderEnabled(true); err != nil {
	// 	log.Fatal(err)
	// }
	headers, err := bor.HeaderEnabled()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Header enabled: %+v\n", headers)

	//

	dest, err := bor.SetDestination("127.0.0.1:2288")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "Destination: %s\n", dest)

	addr, err := net.ResolveUDPAddr("udp", dest)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// rd := borip.NewPacketReader(conn, headers)
	// go func() {
	// 	samples := make([]complex128, 65536)
	// 	// lastIQ := complex(float64(0.0), float64(0.0))
	// 	lastT := time.Now()
	// 	for {
	// 		n, err := rd.ReadSamples(samples)
	// 		if err != nil {
	// 			log.Fatal(err)
	// 		}
	// 		if headers {
	// 			fmt.Fprintf(os.Stderr, "%+v\n", rd.Header())
	// 		}
	// 		t := time.Now()
	// 		rate := float64(n) / (float64(t.Sub(lastT).Nanoseconds()) / 1e9)
	// 		fmt.Printf("Actual rate: %f\n", rate)
	// 		lastT = t

	// 		// for _, iq := range samples {
	// 		// 	pcm := polarDiscriminant(iq, lastIQ)
	// 		// 	lastIQ = iq
	// 		// 	_ = pcm
	// 		// 	// fmt.Printf(" %f", pcm)
	// 		// 	pcm16 := int16(pcm * 16384)
	// 		// 	binary.Write(os.Stdout, binary.LittleEndian, pcm16)
	// 		// }
	// 		// fmt.Println()

	// 		// fm->pre_r = fm->signal[fm->signal_len - 2];
	// 		// fm->pre_j = fm->signal[fm->signal_len - 1];
	// 		// fm->signal2_len = fm->signal_len/2;
	// 		// fmt.Printf("%d:", n)
	// 		// if n > 4 {
	// 		// 	n = 4
	// 		// }
	// 		// for i := 0; i < n; i++ {
	// 		// 	fmt.Printf(" %+v", samples[i])
	// 		// }
	// 		// fmt.Println()
	// 	}
	// }()

	rd := borip.NewPacketReader(conn, headers)
	go func() {
		wr, err := os.Create("samples.bin")
		if err != nil {
			log.Fatal(err)
		}
		defer wr.Close()
		samples := make([]complex128, 65536)
		for {
			n, err := rd.ReadSamples(samples)
			if err != nil {
				log.Fatal(err)
			}
			for i := 0; i < n; i++ {
				if err := binary.Write(wr, binary.LittleEndian, float32(real(samples[i]))); err != nil {
					log.Fatal(err)
				}
				if err := binary.Write(wr, binary.LittleEndian, float32(imag(samples[i]))); err != nil {
					log.Fatal(err)
				}
			}
			if err := wr.Sync(); err != nil {
				log.Fatal(err)
			}
		}
	}()

	bor.Go()

	time.Sleep(time.Second * 30)

	bor.Close()
}
