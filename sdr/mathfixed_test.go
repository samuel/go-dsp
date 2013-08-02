package sdr

import (
	"testing"
)

func BenchmarkFastAtan2Fixed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		switch i & 3 {
		case 0:
			FastAtan2Fixed(10000, 10000)
		case 1:
			FastAtan2Fixed(-10000, 10000)
		case 2:
			FastAtan2Fixed(-10000, -10000)
		case 3:
			FastAtan2Fixed(10000, -10000)
		}
	}
}
