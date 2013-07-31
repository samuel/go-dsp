package rtl

import (
	"testing"
)

func TestBasics(t *testing.T) {
	if devCount := GetDeviceCount(); devCount < 0 {
		t.Fatal("GetDeviceCount failed")
	} else {
		t.Logf("Device count: %d", devCount)
	}
}
