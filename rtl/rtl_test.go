package rtl

import (
	"testing"
)

func TestBasics(t *testing.T) {
	devCount := GetDeviceCount()
	if devCount < 0 || devCount > 16 {
		t.Fatal("GetDeviceCount failed")
	}
	t.Logf("Device count: %d", devCount)

	for i := 0; i < devCount; i++ {
		name := GetDeviceName(i)
		if name == "" {
			t.Fatalf("Failed to get device name for index %d", i)
		}

		manufact, product, serial, err := GetDeviceUSBStrings(i)
		if err != nil {
			t.Fatalf("GetDeviceUSBStrings failed: %+v", err)
		}

		t.Logf("Device %d: %s", i, name)
		t.Logf("\tManufacturer: %s", manufact)
		t.Logf("\tProduct: %s", product)
		t.Logf("\tSerial: %s", serial)

		dev, err := Open(i)
		if err != nil {
			t.Fatalf("Open failed: %+v", err)
		}
		t.Logf("\tTuner Type: %s", dev.GetTunerType())
		if gains, err := dev.GetTunerGains(); err != nil {
			t.Errorf("GetTunerGains failed: %+v", err)
		} else {
			t.Logf("\tTuner Gains: %+v", gains)
		}
		if err := dev.Close(); err != nil {
			t.Fatalf("Close failed: %+v", err)
		}
	}
}
