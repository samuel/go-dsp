// Package rtl provides bindings to the rtl-sdr library.
package rtl

// #cgo LDFLAGS: -lrtlsdr
// #include <stdlib.h>
// #include <rtl-sdr.h>
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

var (
	ErrFailed           = errors.New("rtl: operation failed")
	ErrNoDevices        = errors.New("rtl: no devices")
	ErrNoMatchingDevice = errors.New("rtl: no matching device")
)

type TunerType int

const (
	TunerTypeUnknown TunerType = C.RTLSDR_TUNER_UNKNOWN
	TunerTypeE4000   TunerType = C.RTLSDR_TUNER_E4000
	TunerTypeFC0012  TunerType = C.RTLSDR_TUNER_FC0012
	TunerTypeFC0013  TunerType = C.RTLSDR_TUNER_FC0013
	TunerTypeFC2580  TunerType = C.RTLSDR_TUNER_FC2580
	TunerTypeR820T   TunerType = C.RTLSDR_TUNER_R820T
)

var (
	tunerTypeNames = map[TunerType]string{
		TunerTypeUnknown: "Unknown",
		TunerTypeE4000:   "E4000",
		TunerTypeFC0012:  "FC0012",
		TunerTypeFC0013:  "FC0013",
		TunerTypeFC2580:  "FC2580",
		TunerTypeR820T:   "R820T",
	}
)

func (tt TunerType) String() string {
	if name := tunerTypeNames[tt]; name == "" {
		return "Other"
	} else {
		return name
	}
}

type Device struct {
	cDev *C.rtlsdr_dev_t
}

func GetDeviceCount() int {
	return int(C.rtlsdr_get_device_count())
}

func GetDeviceName(index int) string {
	cName := C.rtlsdr_get_device_name(C.uint32_t(index))
	return C.GoString(cName)
}

// Get USB device strings.
// Return manufacturer, product name, and serial number
func GetDeviceUSBStrings(index int) (string, string, string, error) {
	var manufact [256]C.char
	var product [256]C.char
	var serial [256]C.char
	if C.rtlsdr_get_device_usb_strings(C.uint32_t(index), (*C.char)(&manufact[0]), (*C.char)(&product[0]), (*C.char)(&serial[0])) != 0 {
		return "", "", "", ErrFailed
	}
	return C.GoString((*C.char)(&manufact[0])), C.GoString((*C.char)(&product[0])), C.GoString((*C.char)(&serial[0])), nil
}

// Get device index by USB serial string descriptor.
func GetIndexBySerial(serial string) (int, error) {
	cSerial := C.CString(serial)
	defer C.free(unsafe.Pointer(cSerial))
	res := C.rtlsdr_get_index_by_serial(cSerial)
	if res < 0 {
		switch res {
		case -1:
			// Name is NULL. Shouldn't ever happen.
			return -1, ErrFailed
		case -2:
			return -1, ErrNoDevices
		case -3:
			return -1, ErrNoMatchingDevice
		default:
			return -1, ErrFailed
		}
	}
	return int(res), nil
}

func Open(index int) (*Device, error) {
	dev := &Device{}
	if C.rtlsdr_open(&dev.cDev, C.uint32_t(index)) < 0 {
		return nil, ErrFailed
	}
	runtime.SetFinalizer(dev, func(dev *Device) { dev.Close() })
	return dev, nil
}

func (dev *Device) Close() error {
	if dev.cDev != nil {
		if C.rtlsdr_close(dev.cDev) < 0 {
			return ErrFailed
		}
		dev.cDev = nil
	}
	return nil
}

// Get actual frequency the device is tuned to in Hz.
func (dev *Device) GetCenterFreq() (uint, error) {
	freq := C.rtlsdr_get_center_freq(dev.cDev)
	if freq == 0 {
		return 0, ErrFailed
	}
	return uint(freq), nil
}

func (dev *Device) SetCenterFreq(freq uint) error {
	if C.rtlsdr_set_center_freq(dev.cDev, C.uint32_t(freq)) != 0 {
		return ErrFailed
	}
	return nil
}

func (dev *Device) GetTunerType() TunerType {
	return TunerType(C.rtlsdr_get_tuner_type(dev.cDev))
}

// Get a list of gains supported by the tuner.
func (dev *Device) GetTunerGains() ([]int, error) {
	nGains := C.rtlsdr_get_tuner_gains(dev.cDev, nil)
	if nGains <= 0 {
		return nil, ErrFailed
	}
	cGains := make([]C.int, nGains)
	C.rtlsdr_get_tuner_gains(dev.cDev, &cGains[0])
	gains := make([]int, nGains)
	for i := 0; i < len(gains); i++ {
		gains[i] = int(cGains[i])
	}
	return gains, nil
}

// Set the gain for the device.
// Manual gain mode must be enabled for this to work.
func (dev *Device) SetTunerGain(gain int) error {
	if C.rtlsdr_set_tuner_gain(dev.cDev, C.int(gain)) < 0 {
		return ErrFailed
	}
	return nil
}

// Get actual gain the device is configured to in tength of a dB (115 means 11.5 dB)
func (dev *Device) GetTunerGain() (int, error) {
	if gain := C.rtlsdr_get_tuner_gain(dev.cDev); gain == 0 {
		return 0, ErrFailed
	} else {
		return int(gain), nil
	}
}

// Set the intermediate frequency gain for the device.
// - stage: intermediate frequency gain stage number (1 to 6 for E4000)
// - gain: tenths of a dB, -30 means -3.0 dB
func (dev *Device) SetTunerIfGain(stage, gain int) error {
	if C.rtlsdr_set_tuner_if_gain(dev.cDev, C.int(stage), C.int(gain)) < 0 {
		return ErrFailed
	}
	return nil
}

// Set the gain mode (automatic/manual) for the device.
// Manual gain mode must be enabled for the gain setter function to work.
func (dev *Device) SetTunerGainMode(manual bool) error {
	cManual := C.int(0)
	if manual {
		cManual = 1
	}
	if C.rtlsdr_set_tuner_gain_mode(dev.cDev, cManual) != 0 {
		return ErrFailed
	}
	return nil
}

// select the baseband filters according to the requested sample rate
func (dev *Device) SetSampleRate(rate uint) error {
	if C.rtlsdr_set_sample_rate(dev.cDev, C.uint32_t(rate)) != 0 {
		return ErrFailed
	}
	return nil
}
