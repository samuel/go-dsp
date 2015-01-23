// Package rtl provides bindings to the rtl-sdr library.
package rtl

// #cgo LDFLAGS: -lrtlsdr
// #cgo darwin CFLAGS: -I/usr/local/include
// #cgo darwin LDFLAGS: -L/usr/local/lib
// #cgo pkg-config: libusb-1.0
// #include <stdlib.h>
// #include <rtl-sdr.h>
// #include <libusb.h>
// #include "exports.h"
import "C"

import (
	"errors"
	"log"
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

type ErrLibUSB int

func (e ErrLibUSB) Error() string {
	return C.GoString(C.libusb_error_name(C.int(e)))
}

func (tt TunerType) String() string {
	if name := tunerTypeNames[tt]; name == "" {
		return "Other"
	} else {
		return name
	}
}

// Return true to stop the async loop
type AsyncCallback func(buf []byte) bool

type asyncCallbackContext struct {
	cb  AsyncCallback
	dev *Device
}

//export cbAsyncGo
func cbAsyncGo(buf *C.uchar, size C.uint32_t, ctx unsafe.Pointer) {
	cbCtx := (*asyncCallbackContext)(ctx)

	goBuf := (*[1 << 30]byte)(unsafe.Pointer(buf))[:size:size]
	if cbCtx.cb(goBuf) {
		C.rtlsdr_cancel_async(cbCtx.dev.cDev)
		cbCtx.dev.callbackCtx = nil
	}
}

type Device struct {
	cDev        *C.rtlsdr_dev_t
	callbackCtx *asyncCallbackContext
}

func GetDeviceCount() int {
	return int(C.rtlsdr_get_device_count())
}

func GetDeviceName(index int) string {
	cName := C.rtlsdr_get_device_name(C.uint32_t(index))
	return C.GoString(cName)
}

// GetDeviceUSBStrings returns the USB device strings.
func GetDeviceUSBStrings(index int) (manufacturer, productName, serialNumber string, err error) {
	var manufact [256]C.char
	var prod [256]C.char
	var ser [256]C.char
	if C.rtlsdr_get_device_usb_strings(C.uint32_t(index), (*C.char)(&manufact[0]), (*C.char)(&prod[0]), (*C.char)(&ser[0])) != 0 {
		return "", "", "", ErrFailed
	}
	return C.GoString((*C.char)(&manufact[0])), C.GoString((*C.char)(&prod[0])), C.GoString((*C.char)(&ser[0])), nil
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

/*!
 * Set crystal oscillator frequencies used for the RTL2832 and the tuner IC.
 *
 * Usually both ICs use the same clock. Changing the clock may make sense if
 * you are applying an external clock to the tuner or to compensate the
 * frequency (and samplerate) error caused by the original (cheap) crystal.
 *
 * NOTE: Call this function only if you fully understand the implications.
 *
 * \param rtl_freq frequency value used to clock the RTL2832 in Hz
 * \param tuner_freq frequency value used to clock the tuner IC in Hz
 */
func (dev *Device) SetXtalFreq(rtlFreq, tunerFreq uint) error {
	if C.rtlsdr_set_xtal_freq(dev.cDev, C.uint32_t(rtlFreq), C.uint32_t(tunerFreq)) != 0 {
		return ErrFailed
	}
	return nil
}

// Get crystal oscillator frequencies used for the RTL2832 and the tuner IC.
//
// Usually both ICs use the same clock.
//
// Returns frequency value used to clock the RTL2832 in Hz and
// frequency value used to clock the tuner IC in Hz.
func (dev *Device) GetXtalFreq() (uint, uint, error) {
	var rtlFreq, tunerFreq C.uint32_t
	if C.rtlsdr_get_xtal_freq(dev.cDev, &rtlFreq, &tunerFreq) != 0 {
		return 0, 0, ErrFailed
	}
	return uint(rtlFreq), uint(tunerFreq), nil
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

// Enable or disable the internal digital AGC of the RTL2832.
func (dev *Device) SetAGCMode(enabled bool) error {
	cEnabled := C.int(0)
	if enabled {
		cEnabled = 1
	}
	if C.rtlsdr_set_agc_mode(dev.cDev, cEnabled) != 0 {
		return ErrFailed
	}
	return nil
}

// Select the baseband filters according to the requested sample rate
func (dev *Device) SetSampleRate(rate uint) error {
	if C.rtlsdr_set_sample_rate(dev.cDev, C.uint32_t(rate)) != 0 {
		return ErrFailed
	}
	return nil
}

// Get actual sample rate the device is configured to
func (dev *Device) GetSampleRate() (int, error) {
	if sampleRate := C.rtlsdr_get_sample_rate(dev.cDev); sampleRate == 0 {
		return 0, ErrFailed
	} else {
		return int(sampleRate), nil
	}
}

func (dev *Device) ResetBuffer() error {
	if C.rtlsdr_reset_buffer(dev.cDev) < 0 {
		return ErrFailed
	}
	return nil
}

func (dev *Device) Read(buf []byte) (int, error) {
	var nRead C.int
	if res := C.rtlsdr_read_sync(dev.cDev, unsafe.Pointer(&buf[0]), C.int(len(buf)), &nRead); res != 0 {
		return 0, ErrLibUSB(int(res))
	}
	return int(nRead), nil
}

type buffer struct {
	bytes []byte
	size  int
}

func (dev *Device) ReadAsyncUsingSync(nBuffers, bufferSize int, cb AsyncCallback) error {
	bufferSize &^= 1
	bufferCache := make(chan buffer, nBuffers)
	sampleChan := make(chan buffer, nBuffers)

	for i := 0; i < nBuffers; i++ {
		bufferCache <- buffer{bytes: make([]byte, bufferSize)}
	}

	go func() {
		for {
			buf, ok := <-sampleChan
			if !ok {
				close(bufferCache)
				cb(nil)
				break
			}
			if cb(buf.bytes[:buf.size]) {
				close(bufferCache)
				break
			}
			bufferCache <- buf
		}
	}()

	go func() {
		for {
			buf, ok := <-bufferCache
			if !ok {
				break
			}
			n, err := dev.Read(buf.bytes)
			if err != nil {
				close(sampleChan)
				break
			}
			select {
			case sampleChan <- buffer{bytes: buf.bytes, size: n}:
			default:
				log.Print("dropped packet")
			}
		}
	}()

	return nil
}

func (dev *Device) ReadAsync(nBuffers, bufferSize int, cb AsyncCallback) error {
	go func() {
		dev.callbackCtx = &asyncCallbackContext{
			cb:  cb,
			dev: dev,
		}
		C.rtlsdr_read_async(dev.cDev, (*[0]byte)(unsafe.Pointer(C.cbAsyncPtr)), unsafe.Pointer(dev.callbackCtx), C.uint32_t(nBuffers), C.uint32_t(bufferSize))
	}()
	return nil
}
