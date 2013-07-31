// Package rtl provides bindings to the rtl-sdr library.
package rtl

// #cgo LDFLAGS: -lrtlsdr
// #include <stdlib.h>
// #include <rtl-sdr.h>
import "C"

import (
	"errors"
	"unsafe"
)

var (
	ErrFailed           = errors.New("rtl: operation failed")
	ErrNoDevices        = errors.New("rtl: no devices")
	ErrNoMatchingDevice = errors.New("rtl: no matching device")
)

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
