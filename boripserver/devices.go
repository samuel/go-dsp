package main

// TODO:
// * keep device list up to date (handle inserting/removing new devices)

import (
	"errors"
	"log"
	"sync"

	"github.com/samuel/go-rtlsdr/rtl"
)

var (
	ErrDeviceNotAvailable = errors.New("device not available")
)

type device struct {
	name     string
	rtlIndex int

	mutex         sync.RWMutex
	rtlDev        *rtl.Device
	inUse         bool
	sendCloseChan chan bool
}

var (
	defaultDevice string

	devicesMutex sync.RWMutex
	devices      map[string]*device
)

func init() {
	devices = make(map[string]*device)

	count := rtl.GetDeviceCount()
	for i := 0; i < count; i++ {
		name := rtl.GetDeviceName(i)
		if name == "" {
			log.Printf("RTL returned a blank name for index %d", i)
		} else {
			// TODO: handle non-unique device names
			if defaultDevice == "" {
				defaultDevice = name
			}
			devices[name] = &device{
				name:     name,
				rtlIndex: i,
			}
		}
	}
}

func deviceList() []*device {
	devicesMutex.RLock()
	defer devicesMutex.RUnlock()

	devs := make([]*device, 0, len(devices))
	for _, dev := range devices {
		dev.mutex.Lock()
		if !dev.inUse {
			devs = append(devs, dev)
		}
		dev.mutex.Unlock()
	}
	return devs
}

func (dev *device) open() error {
	dev.mutex.Lock()
	defer dev.mutex.Unlock()
	if dev.inUse {
		return ErrDeviceNotAvailable
	}
	rdev, err := rtl.Open(dev.rtlIndex)
	if err != nil {
		return err
	}
	dev.inUse = true
	dev.rtlDev = rdev
	return nil
}

func (dev *device) close() {
	dev.mutex.Lock()
	defer dev.mutex.Unlock()
	if dev.rtlDev == nil {
		return
	}
	dev.rtlDev.Close()
	dev.rtlDev = nil
	dev.inUse = false
}
