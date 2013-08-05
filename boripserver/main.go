package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	// "time"

	"github.com/samuel/go-sdr/rtl"
)

const (
	debug = true

	defaultPort       = 28888
	eol               = "\n"
	samplesPerPacket  = 4096
	defaultCenterFreq = 144.1e6
	defaultSampleRate = 1000000
	// deviceCacheUpdateInterval = time.Second * 60

	cmdAntenna = "ANTENNA"
	cmdDevice  = "DEVICE"
	cmdFreq    = "FREQ"
	cmdGain    = "GAIN"
	cmdRate    = "RATE"

	resOK      = "OK"
	resFail    = "FAIL"
	resUnknown = "UNKNOWN"
	resDevice  = "DEVICE"
)

var (
	// Keep track of active devices
	openDevicesMutex sync.RWMutex
	openDevices      map[*rtl.Device]*device
	// deviceCacheMutex   sync.RWMutex
	// deviceCache        []*device
	// deviceCacheUpdated time.Time
)

type device struct {
	clients       map[*client]bool
	sendCloseChan chan bool
}

// func sampleSendLoop(dev *device) {

// }

func registerClientDevice(cli *client) {
	openDevicesMutex.Lock()
	defer openDevicesMutex.Unlock()
	if openDevices == nil {
		openDevices = make(map[*rtl.Device]*device)
	}
	dev := openDevices[cli.dev]
	if dev == nil {
		openDevices[cli.dev] = &device{
			clients: map[*client]bool{cli: true},
		}
	} else {
		dev.clients[cli] = true
	}
}

func unregisterClientDevice(cli *client) {
	openDevicesMutex.Lock()
	defer openDevicesMutex.Unlock()
	device := openDevices[cli.dev]
	if device == nil {
		log.Printf("No device struct")
	} else {
		if _, ok := device.clients[cli]; ok {
			delete(device.clients, cli)
			if len(device.clients) == 0 {
				delete(openDevices, cli.dev)
				cli.dev.Close()
			}
		} else {
			log.Println("Client not found when unregistering")
		}
	}
	cli.dev = nil
}

type client struct {
	conn net.Conn
	rd   *bufio.Reader
	wr   *bufio.Writer

	dev *rtl.Device
}

// func devices() []*device {
// 	deviceCacheMutex.RLock()
// 	if time.Since(deviceCacheUpdated).Nanoseconds() > deviceCacheUpdateInterval {
// 		deviceCacheMutex.RUnlock()
// 		deviceCacheMutex.Lock()
// 		deviceCacheUpdated = time.Now()
// 		count := rtl.GetDeviceCount()
// 		devices := make([]*device, count)
// 		for i := 0; i < count; i++ {
// 			if name, err := rtl.GetDeviceName(i); err != nil {
// 				log.Fatalf("Failed to get name for device index %d", i)
// 				continue
// 			}
// 		}
// 		deviceCacheMutex.Unlock()
// 		return devices
// 	} else {
// 		devices := deviceCache
// 		deviceCacheMutex.RUnlock()
// 		return devices
// 	}
// }

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

// 2013/08/05 21:49:08 CLIENT: DEVICE RTL tuner=e4k

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
			if curFreq, err := cli.dev.GetCenterFreq(); err != nil {
				return cli.sendResponse(cmd, "-", "failed to get frequency")
			} else {
				return cli.sendResponse(cmd, strconv.FormatUint(uint64(curFreq), 10))
			}
		}
		if freq, err := strconv.ParseFloat(args[0], 64); err != nil {
			return cli.sendResponse(cmd, resFail, "invalid format for frequency -- expected float")
		} else {
			if err := cli.dev.SetCenterFreq(uint(freq)); err != nil {
				return cli.sendResponse(cmd, resFail, "failed to set frequency")
			} else {
				if curFreq, err := cli.dev.GetCenterFreq(); err != nil {
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
			if rate, err := cli.dev.GetSampleRate(); err != nil {
				return cli.sendResponse(cmd, "-", "failed to get sample rate")
			} else {
				return cli.sendResponse(cmd, strconv.Itoa(rate))
			}
		}
		if rate, err := strconv.ParseFloat(args[0], 64); err != nil {
			return cli.sendResponse(cmd, resFail, "invalid format for sample rate -- expected float")
		} else {
			if err := cli.dev.SetSampleRate(uint(rate)); err != nil {
				return cli.sendResponse(cmd, resFail, "failed to set sample rate")
			} else {
				if curRate, err := cli.dev.GetSampleRate(); err != nil {
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
			if gain, err := cli.dev.GetTunerGain(); err != nil {
				return cli.sendResponse(cmd, "-", "failed to get gain")
			} else {
				return cli.sendResponse(cmd, strconv.Itoa(gain))
			}
		}
		if gain, err := strconv.ParseFloat(args[0], 64); err != nil {
			return cli.sendResponse(cmd, resFail, "invalid format for gain -- expected float")
		} else {
			if err := cli.dev.SetTunerGain(int(gain)); err != nil {
				return cli.sendResponse(cmd, resFail, "failed to set gain")
			} else {
				if curGain, err := cli.dev.GetTunerGain(); err != nil {
					return cli.sendResponse(cmd, resFail, "failed to get gain")
				} else {
					return cli.sendResponse(cmd, resOK, strconv.FormatUint(uint64(curGain), 10))
				}
			}
		}
	case cmdDevice:
		hint := "-"
		if len(args) > 0 {
			hint = args[0]
		}
		switch hint {
		case "-", "rtl": // default UHD device
			if cli.dev != nil {
				unregisterClientDevice(cli)
				cli.dev = nil
			}

			devIndex := 0
			if deviceName := rtl.GetDeviceName(devIndex); deviceName == "" {
				if err := cli.sendResponse(cmd, "-", "no device available"); err != nil {
					return err
				}
			} else {
				if dev, err := rtl.Open(devIndex); err != nil {
					log.Printf("Failed to open device: %s", err.Error())
					if err := cli.sendResponse(cmd, "-", "failed to open device"); err != nil {
						return err
					}
				} else {
					cli.dev = dev
					registerClientDevice(cli)
					dev.SetCenterFreq(defaultCenterFreq)
					dev.SetSampleRate(defaultSampleRate)
					minGain := 0.0
					maxGain := 1.0
					gainStep := 1.0
					gains, err := dev.GetTunerGains()
					if err != nil {
						log.Printf("Failed to get tuner gains: %s", err.Error())
					} else {
						minGain = float64(gains[0])
						maxGain = float64(gains[len(gains)-1])
						// TODO: gainStep
					}
					_, tunerFreq, err := dev.GetXtalFreq()
					if err != nil {
						log.Printf("Failed to get tuner frequency: %s", err.Error())
					} else {
						tunerFreq = 0
					}
					// <DEVICE NAME>|<MIN GAIN>|<MAX GAIN>|<GAIN STEP>|<FPGA FREQ IN HZ>|<COMPLEX SAMPLE PAIRS PER PACKET>|<CSV LIST OF VALID ANTENNAS>[|<DEVICE SERIAL NUMBER>]
					if err := cli.sendResponse(cmd, fmt.Sprintf("%s|%f|%f|%f|%f|%d|default", deviceName, minGain, maxGain, gainStep, float64(tunerFreq), samplesPerPacket)); err != nil {
						return err
					}
				}
			}
		case "!": // release current device
			if cli.dev != nil {
				unregisterClientDevice(cli)
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
	if err := cli.loop(); err != nil && err != io.EOF {
		log.Printf("Client handling error: %s", err.Error())
	}
	if cli.dev != nil {
		unregisterClientDevice(cli)
	}
}

func main() {
	ln, err := net.Listen("tcp", "0.0.0.0:28888")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err.Error())
			continue
		}
		go handleConnection(conn)
	}
}
