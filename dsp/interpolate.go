package dsp

import "math"

// Hermite4p3o interpolates using 4-point, 3rd-order Hermite (x-form)
func Hermite4p3o(samples []float64, x float64) float64 {
	xi := int(x)

	var s [4]float64
	for i := -1; i <= 2; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[i+1] = samples[j]
		}
	}

	x = x - math.Floor(x)
	c0 := s[1]
	c1 := 1 / 2.0 * (s[2] - s[0])
	c2 := s[0] - 5/2.0*s[1] + 2*s[2] - 1/2.0*s[3]
	c3 := 1/2.0*(s[3]-s[0]) + 3/2.0*(s[1]-s[2])
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

	x = x - float32(math.Floor(float64(x)))
	c0 := s[1]
	c1 := 1 / 2.0 * (s[2] - s[0])
	c2 := s[0] - 5/2.0*s[1] + 2*s[2] - 1/2.0*s[3]
	c3 := 1/2.0*(s[3]-s[0]) + 3/2.0*(s[1]-s[2])
	return ((c3*x+c2)*x+c1)*x + c0
}

// Optimal2x6p5o interpolates using optimal 2x (6-point, 5th-order) (z-form)
func Optimal2x6p5o(samples []float64, x float64) float64 {
	xi := int(x)

	var s [6]float64
	for i := -2; i <= 3; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[i+2] = samples[j]
		}
	}

	even1 := s[2+1] + s[2]
	odd1 := s[2+1] - s[2]
	even2 := s[2+2] + s[2-1]
	odd2 := s[2+2] - s[2-1]
	even3 := s[2+3] + s[2-2]
	odd3 := s[2+3] - s[2-2]
	c0 := even1*0.40513396007145713 + even2*0.09251794438424393 + even3*0.00234806603570670
	c1 := odd1*0.28342806338906690 + odd2*0.21703277024054901 + odd3*0.01309294748731515
	c2 := even1*-0.191337682540351941 + even2*0.16187844487943592 + even3*0.02946017143111912
	c3 := odd1*-0.16471626190554542 + odd2*-0.00154547203542499 + odd3*0.03399271444851909
	c4 := even1*0.03845798729588149 + even2*-0.05712936104242644 + even3*0.01866750929921070
	c5 := odd1*0.04317950185225609 + odd2*-0.01802814255926417 + odd3*0.00152170021558204

	z := x - math.Floor(x) - 1/2.0
	return ((((c5*z+c4)*z+c3)*z+c2)*z+c1)*z + c0
}

// Optimal2x6p5oF32 interpolates using optimal 2x (6-point, 5th-order) (z-form)
func Optimal2x6p5oF32(samples []float32, x float32) float32 {
	xi := int(x)

	var s [6]float32
	for i := -2; i <= 3; i++ {
		if j := xi + i; j >= 0 && j < len(samples) {
			s[i+2] = float32(samples[j])
		}
	}

	even1 := s[2+1] + s[2]
	odd1 := s[2+1] - s[2]
	even2 := s[2+2] + s[2-1]
	odd2 := s[2+2] - s[2-1]
	even3 := s[2+3] + s[2-2]
	odd3 := s[2+3] - s[2-2]
	c0 := even1*0.40513396007145713 + even2*0.09251794438424393 + even3*0.00234806603570670
	c1 := odd1*0.28342806338906690 + odd2*0.21703277024054901 + odd3*0.01309294748731515
	c2 := even1*-0.191337682540351941 + even2*0.16187844487943592 + even3*0.02946017143111912
	c3 := odd1*-0.16471626190554542 + odd2*-0.00154547203542499 + odd3*0.03399271444851909
	c4 := even1*0.03845798729588149 + even2*-0.05712936104242644 + even3*0.01866750929921070
	c5 := odd1*0.04317950185225609 + odd2*-0.01802814255926417 + odd3*0.00152170021558204

	x = x - float32(math.Floor(float64(x)))
	z := x - 1/2.0
	return ((((c5*z+c4)*z+c3)*z+c2)*z+c1)*z + c0
}
