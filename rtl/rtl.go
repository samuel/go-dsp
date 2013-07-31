// Package rtl provides bindings to the rtl-sdr library.
package rtl

// #cgo LDFLAGS: -lrtlsdr
// #include <rtl-sdr.h>
import "C"

func GetDeviceCount() int {
	return int(C.rtlsdr_get_device_count())
}

// RTLSDR_API uint32_t rtlsdr_get_device_count(void);

// RTLSDR_API const char* rtlsdr_get_device_name(uint32_t index);
