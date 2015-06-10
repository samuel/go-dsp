package dsp

import "math"

const (
	fixedPi   = 1 << 14
	fixedPi4  = fixedPi / 4
	fixedPi34 = 3 * fixedPi / 4
)

const (
	atanLUTSize = 131072 // 512 KiB
	atanLUTCoef = 8
)

var atanLUT []int

func init() {
	atanLUT = make([]int, atanLUTSize)
	for i := 0; i < atanLUTSize; i++ {
		atanLUT[i] = int(math.Atan(float64(i)/float64(1<<atanLUTCoef)) / math.Pi * (1 << 14))
	}
}

func Atan2LUT(y, x int) int {
	if x == 0 {
		switch {
		case y > 0:
			return 1 << 13
		case y < 0:
			return -(1 << 13)
		}
		return 0
	}

	t := (y << atanLUTCoef) / x
	if t == 0 {
		switch {
		case x > 0:
			return 0
		case y < 0:
			return -(1 << 14)
		}
		return 1 << 14
	}

	if t >= atanLUTSize || -t >= atanLUTSize {
		if y > 0 {
			return 1 << 13
		}
		return -(1 << 13)
	}

	if t > 0 {
		if y > 0 {
			return atanLUT[t]
		}
		return atanLUT[t] - (1 << 14)
	}
	if y > 0 {
		return (1 << 14) - atanLUT[-t]
	}
	return -atanLUT[-t]
}

func FastAtan2Fixed(y, x int) int {
	if x == 0 && y == 0 {
		return 0
	}

	yAbs := y
	if yAbs < 0 {
		yAbs = -yAbs
	}

	var angle int
	if x >= 0 {
		angle = fixedPi4 - fixedPi4*(x-yAbs)/(x+yAbs)
	} else {
		angle = fixedPi34 - fixedPi4*(x+yAbs)/(yAbs-x)
	}
	if y < 0 {
		return -angle
	}
	return angle
}
