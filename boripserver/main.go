package main

// TODO:
// * when settings gain, pick gain closest to an available value as returned by GetRunerGains
// * fix setting (or maybe getting) negative gains

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
)

const (
	debug = true

	defaultPort       = 28888
	eol               = "\n"
	samplesPerPacket  = 4096
	packetsPerBuffer  = 4
	sampleBufferSize  = samplesPerPacket * packetsPerBuffer
	defaultCenterFreq = 144.1e6
	defaultSampleRate = 1000000
	headerSize        = 4
	nBuffers          = 32
	// deviceCacheUpdateInterval = time.Second * 60

	flagNone            = 0x00
	flagHardwareOverrun = 0x01 // Used at hardware interface
	flagNetworkOverrun  = 0x02 // Used at client (network too slow)
	flagBufferOverrun   = 0x04 // Used at client (client consumer too slow)
	flagEmptyPayload    = 0x08 // Reserved
	flagStreamStart     = 0x10 // Used for first packet of newly started stream
	flagStreamEnd       = 0x20 // Reserved (TO DO: Server sends BF_EMPTY_PAYLOAD | BF_STREAM_END)
	flagBufferUnderrun  = 0x40 // Used at hardware interface
	flagHardwareTimeout = 0x80 // Used at hardware interface

	cmdAntenna = "ANTENNA"
	cmdDest    = "DEST"
	cmdDevice  = "DEVICE"
	cmdFreq    = "FREQ"
	cmdGain    = "GAIN"
	cmdGo      = "GO"
	cmdRate    = "RATE"
	cmdStop    = "STOP"

	resOK      = "OK"
	resFail    = "FAIL"
	resUnknown = "UNKNOWN"
	resDevice  = "DEVICE"
)

var (
	flagCpuProfile = flag.Bool("profile.cpu", false, "Enable CPU profiling")
)

var (
	ErrNoDestination = errors.New("no destination")
)

type client struct {
	conn      net.Conn
	rd        *bufio.Reader
	wr        *bufio.Writer
	dest      *net.UDPAddr
	dev       *device
	closeChan chan bool
}

func (cli *client) sendResponse(cmd string, args ...string) error {
	str := cmd
	if len(args) > 0 {
		str += " " + strings.Join(args, " ")
	}
	if debug {
		log.Printf("SERVER: %s", str)
	}
	_, err := cli.wr.WriteString(str + eol)
	if err == nil {
		err = cli.wr.Flush()
	}
	return err
}

func (cli *client) handleCommand(cmd string, args []string) error {
	switch cmd {
	default:
		if err := cli.sendResponse(cmd, resUnknown); err != nil {
			return err
		}
	case cmdFreq:
		if cli.dev == nil {
			return cli.sendResponse(cmd, resDevice, "no active device")
		}
		if len(args) == 0 {
			if curFreq, err := cli.dev.rtlDev.GetCenterFreq(); err != nil {
				return cli.sendResponse(cmd, "-", "failed to get frequency")
			} else {
				return cli.sendResponse(cmd, strconv.FormatUint(uint64(curFreq), 10))
			}
		}
		if freq, err := strconv.ParseFloat(args[0], 64); err != nil {
			return cli.sendResponse(cmd, resFail, "invalid format for frequency -- expected float")
		} else {
			if err := cli.dev.rtlDev.SetCenterFreq(uint(freq)); err != nil {
				return cli.sendResponse(cmd, resFail, "failed to set frequency")
			} else {
				if curFreq, err := cli.dev.rtlDev.GetCenterFreq(); err != nil {
					return cli.sendResponse(cmd, resFail, "failed to get frequency")
				} else {
					return cli.sendResponse(cmd, resOK, fmt.Sprintf("%f %d %f %f", freq, curFreq, 0.0, 0.0))
				}
			}
		}
	case cmdAntenna:
		if cli.dev == nil {
			return cli.sendResponse(cmd, resDevice, "no active device")
		}
		if len(args) > 0 {
			return cli.sendResponse(cmd, resOK)
		} else {
			return cli.sendResponse(cmd, resOK, "default")
		}
	case cmdRate:
		if cli.dev == nil {
			return cli.sendResponse(cmd, resDevice, "no active device")
		}
		if len(args) == 0 {
			if rate, err := cli.dev.rtlDev.GetSampleRate(); err != nil {
				return cli.sendResponse(cmd, "-", "failed to get sample rate")
			} else {
				return cli.sendResponse(cmd, strconv.Itoa(rate))
			}
		}
		if rate, err := strconv.ParseFloat(args[0], 64); err != nil {
			return cli.sendResponse(cmd, resFail, "invalid format for sample rate -- expected float")
		} else {
			if err := cli.dev.rtlDev.SetSampleRate(uint(rate)); err != nil {
				return cli.sendResponse(cmd, resFail, "failed to set sample rate")
			} else {
				if curRate, err := cli.dev.rtlDev.GetSampleRate(); err != nil {
					return cli.sendResponse(cmd, resFail, "failed to get sample rate")
				} else {
					return cli.sendResponse(cmd, resOK, strconv.FormatUint(uint64(curRate), 10))
				}
			}
		}
	case cmdGain:
		if cli.dev == nil {
			return cli.sendResponse(cmd, resDevice, "no active device")
		}
		if len(args) == 0 {
			if gain, err := cli.dev.rtlDev.GetTunerGain(); err != nil {
				return cli.sendResponse(cmd, "-", "failed to get gain")
			} else {
				return cli.sendResponse(cmd, strconv.Itoa(gain))
			}
		}
		if gain, err := strconv.ParseFloat(args[0], 64); err != nil {
			return cli.sendResponse(cmd, resFail, "invalid format for gain -- expected float")
		} else {
			if err := cli.dev.rtlDev.SetTunerGain(int(gain)); err != nil {
				return cli.sendResponse(cmd, resFail, "failed to set gain")
			} else {
				if curGain, err := cli.dev.rtlDev.GetTunerGain(); err != nil {
					return cli.sendResponse(cmd, resFail, "failed to get gain")
				} else {
					return cli.sendResponse(cmd, resOK, strconv.FormatUint(uint64(curGain), 10))
				}
			}
		}
	case cmdDest:
		if len(args) == 0 {
			if cli.dest == nil {
				return cli.sendResponse(cmd, "-", "no DEST set")
			} else {
				return cli.sendResponse(cmd, cli.dest.String())
			}
		}
		addr, err := net.ResolveUDPAddr("udp", args[0])
		if err != nil {
			log.Printf("Failed to resolve UDP address %s: %s", args[0], err.Error())
			return cli.sendResponse(cmd, resFail, "failed to resolve address")
		} else {
			cli.dest = addr
			return cli.sendResponse(cmd, resOK)
		}
	case cmdGo:
		if cli.dest == nil {
			return cli.sendResponse(cmd, resFail, "no DEST set")
		}
		if cli.dev == nil {
			return cli.sendResponse(cmd, resDevice, "no active device")
		}
		if cli.isStreaming() {
			return cli.sendResponse(cmd, "RUNNING")
		}
		if err := cli.startStreaming(); err != nil {
			return cli.sendResponse(cmd, resFail, err.Error())
		} else {
			return cli.sendResponse(cmd, resOK)
		}
	case cmdStop:
		if !cli.isStreaming() {
			return cli.sendResponse(cmd, "STOPPED")
		}
		cli.stopStreaming()
		return cli.sendResponse(cmd, resOK)
	case cmdDevice:
		hint := "-"
		if len(args) > 0 {
			hint = args[0]
		}
		// DEVICE RTL tuner=e4k
		switch hint {
		case "-": // default UHD device
			if cli.isStreaming() {
				cli.stopStreaming()
			}
			if cli.dev != nil {
				cli.dev.close()
				cli.dev = nil
			}

			devices := deviceList()

			var dev *device
			for _, d := range devices {
				if err := d.open(); err == nil {
					dev = d
					break
				} else if err != ErrDeviceNotAvailable {
					log.Printf("Failed to open device %s: %s", d.name, err.Error())
				}
			}

			if dev == nil {
				return cli.sendResponse(cmd, "-", "no devices available")
			}

			cli.dev = dev
			if err := dev.rtlDev.SetCenterFreq(defaultCenterFreq); err != nil {
				log.Printf("Failed to set default center frequency: %+v", err)
			}
			if err := dev.rtlDev.SetSampleRate(defaultSampleRate); err != nil {
				log.Printf("Failed to set default sample rate: %+v", err)
			}
			if err := dev.rtlDev.SetAGCMode(false); err != nil {
				log.Printf("Failed to diable AGC mode: %+v", err)
			}
			minGain := 0.0
			maxGain := 1.0
			gainStep := 1.0
			gains, err := dev.rtlDev.GetTunerGains()
			if err != nil {
				log.Printf("Failed to get tuner gains: %s", err.Error())
			} else {
				minGain = float64(gains[0])
				maxGain = float64(gains[len(gains)-1])
				gainStep = float64(gains[1] - gains[0])
			}
			// if debug {
			// 	log.Printf("Gains: %+v", gains)
			// }
			_, tunerFreq, err := dev.rtlDev.GetXtalFreq()
			if err != nil {
				log.Printf("Failed to get tuner frequency: %s", err.Error())
			} else {
				tunerFreq = 0
			}
			// <DEVICE NAME>|<MIN GAIN>|<MAX GAIN>|<GAIN STEP>|<FPGA FREQ IN HZ>|<COMPLEX SAMPLE PAIRS PER PACKET>|<CSV LIST OF VALID ANTENNAS>[|<DEVICE SERIAL NUMBER>]
			if err := cli.sendResponse(cmd, fmt.Sprintf("%s|%f|%f|%f|%f|%d|default", dev.name, minGain, maxGain, gainStep, float64(tunerFreq), samplesPerPacket)); err != nil {
				return err
			}
		case "!": // release current device
			if cli.isStreaming() {
				cli.stopStreaming()
			}
			if cli.dev != nil {
				cli.dev.close()
				cli.dev = nil
			}
			if err := cli.sendResponse(cmd, "-"); err != nil {
				return err
			}
		default:
			if err := cli.sendResponse(cmd, "-", "unknown hint"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (cli *client) isStreaming() bool {
	return cli.closeChan != nil
}

func (cli *client) stopStreaming() {
	if !cli.isStreaming() {
		return
	}
	close(cli.closeChan)
}

func (cli *client) startStreaming() error {
	if cli.isStreaming() {
		return nil
	}
	if cli.dest == nil {
		return ErrNoDestination
	}

	conn, err := net.DialUDP("udp", nil, cli.dest)
	if err != nil {
		return err
	}
	if err := cli.dev.rtlDev.ResetBuffer(); err != nil {
		return err
	}

	cli.closeChan = make(chan bool, 1)
	bufOut := make([]byte, headerSize+samplesPerPacket*2*2)
	bufOut[0] = flagStreamStart
	bufOut[1] = 0 // notification: reserved (currently 0)
	first := true
	seq := 0

	// cli.dev.rtlDev.ReadAsyncUsingSync(nBuffers, sampleBufferSize*2, func(buf []byte) bool {
	cli.dev.rtlDev.ReadAsync(nBuffers, sampleBufferSize*2, func(buf []byte) bool {
		if buf == nil {
			cli.closeChan = nil
			return true
		}

		select {
		case _ = <-cli.closeChan:
			cli.closeChan = nil
			return true
		default:
		}

		for bufferOffset := 0; bufferOffset < len(buf); bufferOffset += samplesPerPacket * 2 {
			nValues := len(buf) - bufferOffset
			if nValues > samplesPerPacket*2 {
				nValues = samplesPerPacket * 2
			}

			bufOut[2] = uint8(seq & 0xff)
			bufOut[3] = uint8(seq >> 8)
			seq++

			Ui8toi16(buf[:nValues], bufOut[headerSize:])

			if _, err := conn.Write(bufOut[:headerSize+nValues*2]); err != nil {
				// TODO: what to do if not "connection refused"?
			}

			if first {
				bufOut[0] = 0
				first = false
			}
		}

		return false
	})

	return nil
}

func (cli *client) loop() error {
	if err := cli.sendResponse("DEVICE", "-"); err != nil {
		return err
	}
	for {
		lineBytes, err := cli.rd.ReadSlice('\n')
		if err != nil {
			return err
		}
		line := string(bytes.TrimSpace(lineBytes))
		if len(line) == 0 {
			continue
		}
		if debug {
			log.Printf("CLIENT: %s", line)
		}
		parts := strings.Split(line, " ")
		cmd := strings.ToUpper(parts[0])
		args := parts[1:]
		if err := cli.handleCommand(cmd, args); err != nil {
			return err
		}
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	cli := &client{
		conn: conn,
		wr:   bufio.NewWriter(conn),
		rd:   bufio.NewReader(conn),
	}
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		cli.dest = &net.UDPAddr{
			IP:   addr.IP,
			Zone: addr.Zone,
			Port: defaultPort,
		}
	}
	if err := cli.loop(); err != nil && err != io.EOF {
		log.Printf("Client handling error: %s", err.Error())
	}
	cli.stopStreaming()
	if cli.dev != nil {
		cli.dev.close()
	}
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

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

	ln, err := net.Listen("tcp", "0.0.0.0:28888")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println(err.Error())
				continue
			}
			go handleConnection(conn)
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)
	_ = <-signalChan

	if *flagCpuProfile {
		pprof.StopCPUProfile()
	}
}
