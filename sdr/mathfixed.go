package sdr

const (
	fixedPi   = 1 << 14
	fixedPi4  = fixedPi / 4
	fixedPi34 = 3 * fixedPi / 4
)

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
