package dspviz

import "math"

// The I/Q figures, and the five running sums they share with the stereo pair
// comparison in pair.go.
//
// A quadrature receiver's imbalance and a stereo pair's correlation are the
// same arithmetic asked two different questions, which is why crossSums serves
// both: gain imbalance is 10*log10(<I^2>/<Q^2>), quadrature error is
// asin(<IQ>/sqrt(<I^2><Q^2>)), and a correlation of one is dual mono.

// IQStats are the defects a quadrature receiver has, all closed form from three
// running sums. The same sums describe a pair of ordinary channels, which is why
// this is also how a stereo file is checked: a quadrature error near 90 degrees
// means the two channels are an I/Q pair written as left and right.
type IQStats struct {
	DCI  float64 `json:"dcI"`
	DCQ  float64 `json:"dcQ"`
	DCDB float64 `json:"dcDB"`

	// GainImbalanceDB is I relative to Q; QuadratureErrorDeg is how far their
	// phase difference is from a right angle.
	GainImbalanceDB    float64 `json:"gainImbalanceDB"`
	QuadratureErrorDeg float64 `json:"quadratureErrorDeg"`

	Correlation float64 `json:"correlation"`
	DualMono    bool    `json:"dualMono"`
}

func (b *StatsBuilder) iq() *IQStats {
	n := float64(b.n)
	if n == 0 {
		return nil
	}
	mI, mQ, pI, pQ, pIQ := b.cross.moments()

	iq := &IQStats{DCI: mI, DCQ: mQ, DCDB: DB(math.Hypot(mI, mQ))}
	if pQ > 0 && pI > 0 {
		iq.GainImbalanceDB = dbPower(pI / pQ)
		iq.QuadratureErrorDeg = math.Asin(clamp(pIQ/math.Sqrt(pI*pQ), -1, 1)) * 180 / math.Pi
	}
	// The centered correlation, which is the stereo question rather than the
	// quadrature one: it does not care about a DC offset in either channel.
	iq.Correlation = b.cross.correlation()
	iq.DualMono = iq.Correlation > 0.9999 && math.Abs(iq.GainImbalanceDB) < 0.01
	return iq
}

// crossSums is everything the relationship between two signals needs: five
// running sums giving the correlation, the balance and the phase in closed form,
// none of them growing with the input. One accumulator serves IQStats and
// ChannelPairStats both.
type crossSums struct {
	n                   int64
	sumA, sumB          float64
	sumA2, sumB2, sumAB float64
}

func (c *crossSums) add(a, b float64) {
	c.n++
	c.sumA += a
	c.sumB += b
	c.sumA2 += a * a
	c.sumB2 += b * b
	c.sumAB += a * b
}

// moments returns the two means and the three second moments, all per sample.
func (c *crossSums) moments() (mA, mB, pA, pB, pAB float64) {
	if c.n == 0 {
		return 0, 0, 0, 0, 0
	}
	n := float64(c.n)
	return c.sumA / n, c.sumB / n, c.sumA2 / n, c.sumB2 / n, c.sumAB / n
}

// correlation is the centered Pearson correlation: 1 for identical signals, 0
// for unrelated ones, -1 for one that is the other inverted, and indifferent to
// a DC offset in either.
func (c *crossSums) correlation() float64 {
	mA, mB, pA, pB, pAB := c.moments()
	vA, vB := pA-mA*mA, pB-mB*mB
	if vA <= 0 || vB <= 0 {
		return 0
	}
	return clamp((pAB-mA*mB)/math.Sqrt(vA*vB), -1, 1)
}

func dualMonoNote(q *IQStats) string {
	switch {
	case q.DualMono:
		return ", which is dual mono"
	case math.Abs(math.Abs(q.QuadratureErrorDeg)-90) < 1:
		return ", a quarter cycle apart: these are an I/Q pair on two channels"
	}
	return ""
}
