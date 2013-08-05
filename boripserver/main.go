package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	// "time"

	"github.com/samuel/go-sdr/rtl"
)

const (
	defaultPort      = 28888
	eol              = "\n"
	samplesPerPacket = 4096
	// deviceCacheUpdateInterval = time.Second * 60

	cmdDevice = "DEVICE"

	resOK      = "OK"
	resFail    = "FAIL"
	resUnknown = "UNKNOWN"
	resDevice  = "DEVICE"
)

var (
	// Keep track of active devices
	openDevicesMutex sync.RWMutex
	openDevices      map[*rtl.Device]map[*client]bool
	// deviceCacheMutex   sync.RWMutex
	// deviceCache        []*device
	// deviceCacheUpdated time.Time
)

func registerClientDevice(cli *client) {
	openDevicesMutex.Lock()
	defer openDevicesMutex.Unlock()
	cliMap := openDevices[cli.dev]
	if cliMap == nil {
		openDevices[cli.dev] = map[*client]bool{cli: true}
	} else {
		cliMap[cli] = true
	}
}

func unregisterClientDevice(cli *client) {
	openDevicesMutex.Lock()
	defer openDevicesMutex.Unlock()
	clients := openDevices[cli.dev]
	if _, ok := clients[cli]; ok {
		delete(clients, cli)
		if len(clients) == 0 {
			delete(openDevices, cli.dev)
			cli.dev.Close()
		}
	} else {
		log.Println("Client not found when unregistering")
	}
	cli.dev = nil
}

type client struct {
	conn net.Conn
	rd   *bufio.Reader
	wr   *bufio.Writer

	dev *rtl.Device
}

// type device struct {
// 	name                       string
// 	serial                     string
// 	minGain, maxGain, gainStep float64
// 	fpgaFreq                   float64
// }

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
	_, err := cli.wr.WriteString(str + eol)
	if err == nil {
		err = cli.wr.Flush()
	}
	return err
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
		parts := strings.Split(line, " ")
		cmd := strings.ToUpper(parts[0])
		args := parts[1:]
		switch cmd {
		default:
			if err := cli.sendResponse(cmd, resUnknown); err != nil {
				return err
			}
		case cmdDevice:
			hint := "-"
			if len(args) > 0 {
				hint = args[0]
			}
			switch hint {
			case "-", "rtl": // default UHD device
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
