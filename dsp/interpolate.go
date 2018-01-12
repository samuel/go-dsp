package dsp

import "math"

// Linear interpolates using linear interpolation.
func Linear(samples []float64, x float64) float64 {
	var samp float64
	low := math.Floor(x)
	lowInt := int(low)
	if lowInt < len(samples) {
		lowValue := samples[lowInt]
		highValue := lowValue
		if i := lowInt + 1; i >= len(samples) {
			highValue = 0
		} else {
			highValue = samples[i]
		}
		samp = lowValue + (x-low)*(highValue-lowValue)
	}
	return samp
}

// LinearF32 interpolates using linear interpolation.
func LinearF32(samples []float32, x float32) float32 {
	var samp float32
	low := float32(math.Floor(float64(x)))
	lowInt := int(low)
	if lowInt < len(samples) {
		lowValue := samples[lowInt]
		highValue := lowValue
		if i := lowInt + 1; i >= len(samples) {
			highValue = 0
		} else {
			highValue = samples[i]
		}
		samp = lowValue + (x-low)*(highValue-lowValue)
	}
	return samp
}

// Hermite4p3o interpolates using 4-point, 3rd-order Hermite (x-form)
func Hermite4p3o(samples []float64, x float64) float64 {
	xi := int(x)

	var s [4]float64
	for i := -1; i <= 2; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[i+1] = samples[j]
		}
	}

	x -= math.Floor(x)
	c0 := s[1]
	c1 := 1.0 / 2.0 * (s[2] - s[0])
	c2 := s[0] - 5.0/2.0*s[1] + 2.0*s[2] - 1.0/2.0*s[3]
	c3 := 1.0/2.0*(s[3]-s[0]) + 3.0/2.0*(s[1]-s[2])
	return ((c3*x+c2)*x+c1)*x + c0
}

// Hermite4p3oF32 interpolates using 4-point, 3rd-order Hermite (x-form)
func Hermite4p3oF32(samples []float32, x float32) float32 {
	xi := int(x)

	var s [4]float32
	for i := -1; i <= 2; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[i+1] = float32(samples[j])
		}
	}

	x -= float32(math.Floor(float64(x)))
	c0 := s[1]
	c1 := 1.0 / 2.0 * (s[2] - s[0])
	c2 := s[0] - 5.0/2.0*s[1] + 2.0*s[2] - 1.0/2.0*s[3]
	c3 := 1.0/2.0*(s[3]-s[0]) + 3.0/2.0*(s[1]-s[2])
	return ((c3*x+c2)*x+c1)*x + c0
}

// Optimal2x4p4o interpolates using optimal 2x (4-point, 4th-order) (z-form)
func Optimal2x4p4o(samples []float64, x float64) float64 {
	const middle = 1

	xi := int(x)

	var s [6]float64
	for i := -1; i <= 2; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[middle+i] = samples[j]
		}
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	c0 := even1*0.45645918406487612 + even2*0.04354173901996461
	c1 := odd1*0.47236675362442071 + odd2*0.17686613581136501
	c2 := even1*-0.253674794204558521 + even2*0.25371918651882464
	c3 := odd1*-0.37917091811631082 + odd2*0.11952965967158000
	c4 := even1*0.04252164479749607 + even2*-0.04289144034653719

	z := x - math.Floor(x) - 1.0/2.0
	return (((c4*z+c3)*z+c2)*z+c1)*z + c0
}

// Optimal2x4p4oF32 interpolates using optimal 2x (4-point, 4th-order) (z-form)
func Optimal2x4p4oF32(samples []float32, x float32) float32 {
	const middle = 1

	xi := int(x)

	var s [6]float32
	for i := -1; i <= 2; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[middle+i] = samples[j]
		}
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	c0 := even1*0.45645918406487612 + even2*0.04354173901996461
	c1 := odd1*0.47236675362442071 + odd2*0.17686613581136501
	c2 := even1*-0.253674794204558521 + even2*0.25371918651882464
	c3 := odd1*-0.37917091811631082 + odd2*0.11952965967158000
	c4 := even1*0.04252164479749607 + even2*-0.04289144034653719

	z := x - float32(math.Floor(float64(x))) - 1.0/2.0
	return (((c4*z+c3)*z+c2)*z+c1)*z + c0
}

// Optimal2x6p5o interpolates using optimal 2x (6-point, 5th-order) (z-form)
func Optimal2x6p5o(samples []float64, x float64) float64 {
	const middle = 2
	xi := int(x)

	var s [6]float64
	for i := -2; i <= 3; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[middle+i] = samples[j]
		}
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	even3 := s[middle+3] + s[middle-2]
	odd3 := s[middle+3] - s[middle-2]
	c0 := even1*0.40513396007145713 + even2*0.09251794438424393 + even3*0.00234806603570670
	c1 := odd1*0.28342806338906690 + odd2*0.21703277024054901 + odd3*0.01309294748731515
	c2 := even1*-0.191337682540351941 + even2*0.16187844487943592 + even3*0.02946017143111912
	c3 := odd1*-0.16471626190554542 + odd2*-0.00154547203542499 + odd3*0.03399271444851909
	c4 := even1*0.03845798729588149 + even2*-0.05712936104242644 + even3*0.01866750929921070
	c5 := odd1*0.04317950185225609 + odd2*-0.01802814255926417 + odd3*0.00152170021558204

	z := x - math.Floor(x) - 1.0/2.0
	return ((((c5*z+c4)*z+c3)*z+c2)*z+c1)*z + c0
}

// Optimal2x6p5oF32 interpolates using optimal 2x (6-point, 5th-order) (z-form)
func Optimal2x6p5oF32(samples []float32, x float32) float32 {
	const middle = 2
	xi := int(x)

	var s [6]float32
	for i := -2; i <= 3; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[middle+i] = float32(samples[j])
		}
	}

	even1 := s[middle+1] + s[middle]
	odd1 := s[middle+1] - s[middle]
	even2 := s[middle+2] + s[middle-1]
	odd2 := s[middle+2] - s[middle-1]
	even3 := s[middle+3] + s[middle-2]
	odd3 := s[middle+3] - s[middle-2]
	c0 := even1*0.40513396007145713 + even2*0.09251794438424393 + even3*0.00234806603570670
	c1 := odd1*0.28342806338906690 + odd2*0.21703277024054901 + odd3*0.01309294748731515
	c2 := even1*-0.191337682540351941 + even2*0.16187844487943592 + even3*0.02946017143111912
	c3 := odd1*-0.16471626190554542 + odd2*-0.00154547203542499 + odd3*0.03399271444851909
	c4 := even1*0.03845798729588149 + even2*-0.05712936104242644 + even3*0.01866750929921070
	c5 := odd1*0.04317950185225609 + odd2*-0.01802814255926417 + odd3*0.00152170021558204

	z := x - float32(math.Floor(float64(x))) - 1.0/2.0
	return ((((c5*z+c4)*z+c3)*z+c2)*z+c1)*z + c0
}
