// Package firpm designs linear-phase FIR filters with the Parks-McClellan
// algorithm, which uses the Remez exchange to find the filter whose weighted
// Chebyshev error against a desired response is minimal.
//
// It is a transliteration, statement for statement, of the Fortran program by
// McClellan, Parks and Rabiner kept alongside as remez.fortran -- including its
// two non-convergence exits, which return a filter along with an error rather
// than nothing at all.
//
// The arithmetic is float64 throughout, where the original's grid, weights and
// impulse response are single precision, so this package is more accurate than
// identical. It departs in three further places: math.Acos where the original
// loses digits to a cancellation, reporting a specification it cannot
// approximate instead of returning infinities, and rejecting band edges that
// repeat.
package firpm

import (
	"errors"
	"math"
	"strconv"
)

//go:generate ./testdata/generate.sh

// FilterType selects the class of filter to design.
type FilterType int

const (
	// BandPass designs a symmetric filter approximating the desired
	// magnitude in each band.
	BandPass FilterType = iota
	// Differentiator designs an antisymmetric filter whose response is
	// proportional to frequency, with the weight taken inversely
	// proportional to frequency.
	Differentiator
	// Hilbert designs an antisymmetric filter approximating a 90 degree
	// phase shift.
	Hilbert
)

// Errors reported by Remez when the exchange fails to converge. Both mirror a
// warning-and-carry-on exit of the Fortran, so coefficients are returned
// alongside the error. The original's advice for the second is that the design
// is probably usable but should be verified with an FFT.
var (
	ErrMaxIterations      = errors.New("firpm: reached the maximum number of iterations")
	ErrDeviationDecreased = errors.New("firpm: deviation decreased, probable machine rounding error")

	// ErrIllConditioned reports a specification that cannot be approximated at
	// all, rather than one approximated badly.
	ErrIllConditioned = errors.New("firpm: design is ill-conditioned, coefficients are not finite")
)

const (
	pi  = math.Pi
	pi2 = math.Pi * 2

	// The original's LGRID default and ITRMAX constant.
	defaultGridDensity   = 16
	defaultMaxIterations = 25

	// Bounds that keep the grid arrays allocatable and the arithmetic in int.
	// Far past any useful filter: the Fortran capped length at 128 and grid at
	// 1045.
	maxNumTaps     = 1 << 16
	maxGridDensity = 1 << 10
	maxWorkSize    = 1 << 22
)

// trace reports the interior state of the exchange loop. It is nil outside of
// tests, which compare the trajectory step by step against the Fortran's.
type trace struct {
	iteration func(niter, luck, jchnge int, iext []int)
	deviation func(niter int, dev float64)
	final     func(niter, kkk, luck, jchnge int)
}

/*
 *-----------------------------------------------------------------------
 * FUNCTION: D
 *  FUNCTION TO CALCULATE THE LAGRANGE INTERPOLATION
 *  COEFFICIENTS FOR USE IN THE FUNCTION GEE.
 *-----------------------------------------------------------------------
 */
func lagrangeInterp(k, n, m int, x []float64) float64 {
	retval := 1.0
	q := x[k]

	for l := 1; l <= m; l++ {
		for j := l; j <= n; j += m {
			if j != k {
				retval *= 2.0 * (q - x[j])
			}
		}
	}
	return 1.0 / retval
}

/*
 *-----------------------------------------------------------------------
 * FUNCTION: GEE
 *  FUNCTION TO EVALUATE THE FREQUENCY RESPONSE USING THE
 *  LAGRANGE INTERPOLATION FORMULA IN THE BARYCENTRIC FORM
 *-----------------------------------------------------------------------
 */
func freqEval(k, n int, grid, x, y, ad []float64) float64 {
	d := 0.0
	p := 0.0
	xf := math.Cos(pi2 * grid[k])

	for j := 1; j <= n; j++ {
		c := ad[j] / (xf - x[j])
		d += c
		p += c * y[j]
	}

	return p / d
}

/*
 *-----------------------------------------------------------------------
 * SUBROUTINE: REMEZ
 *  THIS SUBROUTINE IMPLEMENTS THE REMEZ EXCHANGE ALGORITHM
 *  FOR THE WEIGHTED CHEBYSHEV APPROXIMATION OF A CONTINUOUS
 *  FUNCTION WITH A SUM OF COSINES.  INPUTS TO THE SUBROUTINE
 *  ARE A DENSE GRID WHICH REPLACES THE FREQUENCY AXIS, THE
 *  DESIRED FUNCTION ON THIS GRID, THE WEIGHT FUNCTION ON THE
 *  GRID, THE NUMBER OF COSINES, AND AN INITIAL GUESS OF THE
 *  EXTREMAL FREQUENCIES.  THE PROGRAM MINIMIZES THE CHEBYSHEV
 *  ERROR BY DETERMINING THE BEST LOCATION OF THE EXTREMAL
 *  FREQUENCIES (POINTS OF MAXIMUM ERROR) AND THEN CALCULATES
 *  THE COEFFICIENTS OF THE BEST APPROXIMATION.
 *-----------------------------------------------------------------------
 */
func remez(des, grid, wt []float64, ngrid int, iext []int, alpha []float64, nfcns, itrmax, dimsize int, tr *trace) (float64, error) {
	a := make([]float64, dimsize+1)
	p := make([]float64, dimsize+1)
	q := make([]float64, dimsize+1)
	ad := make([]float64, dimsize+1)
	x := make([]float64, dimsize+1)
	y := make([]float64, dimsize+1)

	devl := -1.0
	nz := nfcns + 1
	nzz := nfcns + 2

	var comp, dev, y1 float64
	// niter, luck and jchnge outlive a single pass, exactly as the Fortran's
	// locals do across its GO TO 100, so the trace can report the preceding
	// pass's outcome the way the reference dump does.
	var niter, luck, jchnge int
	var err error

IterationLoop:
	for {
		iext[nzz] = ngrid + 1
		niter++
		if niter > itrmax {
			err = ErrMaxIterations
			break
		}
		if tr != nil {
			tr.iteration(niter, luck, jchnge, iext[1:nzz+1])
		}

		for j := 1; j <= nz; j++ {
			x[j] = math.Cos(grid[iext[j]] * pi2)
		}

		jet := (nfcns-1)/15 + 1
		for j := 1; j <= nz; j++ {
			ad[j] = lagrangeInterp(j, nz, jet, x)
		}

		dnum, dden := 0.0, 0.0
		for j, k := 1, 1.0; j <= nz; j, k = j+1, -k {
			l := iext[j]
			dnum += ad[j] * des[l]
			dden += k * ad[j] / wt[l]
		}
		dev = dnum / dden
		if tr != nil {
			tr.deviation(niter, dev)
		}

		nu := 1.0
		if dev > 0.0 {
			nu = -1.0
		}
		dev = math.Abs(dev) // dev = -nu * dev
		for j, k := 1, nu; j <= nz; j, k = j+1, -k {
			l := iext[j]
			y[j] = des[l] + k*dev/wt[l]
		}
		// The reference continues only when DEV .GT. DEVL, so the exit is
		// the negation of that and not dev <= devl: a NaN deviation, which
		// a degenerate band layout can produce, compares false either way
		// and has to leave the loop rather than iterate on nothing.
		if !(dev > devl) {
			err = ErrDeviationDecreased
			break
		}
		devl = dev

		jchnge = 0
		k1 := iext[1]
		knz := iext[nz]
		klow := 0
		nut := -nu

		down := func(l, j int) {
			for {
				l--
				if l <= klow {
					break
				}
				e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
				if nut*e-comp <= 0.0 {
					break
				}
				comp = nut * e
			}
			klow = iext[j]
			iext[j] = l + 1
			jchnge++
		}

		up := func(l, j, kup int) {
			for {
				l++
				if l >= kup {
					break
				}
				e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
				if nut*e-comp <= 0.0 {
					break
				}
				comp = nut * e
			}
			iext[j] = l - 1
			klow = l - 1
			jchnge++
		}

		/*
		 * SEARCH FOR THE EXTREMAL FREQUENCIES OF THE BEST APPROXIMATION
		 */

		for j := 1; j < nzz; j++ {
			kup := iext[j+1]
			nut = -nut
			if j == 2 {
				y1 = comp
			}
			comp = dev

			l := iext[j] + 1
			if l < kup {
				e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
				if nut*e-comp > 0.0 {
					comp = nut * e
					up(l, j, kup)
					continue
				}
			}

			l--

			for {
				l--
				if l <= klow {
					l = iext[j] + 1
					if jchnge > 0 {
						iext[j] = l - 1
						klow = l - 1
						jchnge++
					} else {
						for {
							l++
							if l >= kup {
								klow = iext[j]
								break
							}
							e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
							if nut*e-comp > 0.0 {
								comp = nut * e
								up(l, j, kup)
								break
							}
						}
					}
					break
				}
				e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
				if nut*e-comp > 0.0 {
					comp = nut * e
					down(l, j)
					break
				}
				if jchnge > 0 {
					klow = iext[j]
					break
				}
			}
		}

		// The loop above leaves with j == nzz, where the Fortran takes the
		// copy YNZ = COMP that scales the search below.
		if k1 > iext[1] {
			k1 = iext[1]
		}
		if knz < iext[nz] {
			knz = iext[nz]
		}

		nut1 := nut
		nut = -nu
		luck = 1
		comp *= 1.00001

		found := false
		for l := 1; l < k1; l++ {
			e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
			if nut*e-comp > 0.0 {
				comp = nut * e
				up(l, nzz, k1)
				found = true

				if comp > y1 {
					y1 = comp
				}
				k1 = iext[nzz]
				break
			}
		}
		if !found {
			luck = 6
		}

		klow = knz
		nut = -nut1
		comp = y1 * 1.00001

		for l := ngrid; l > klow; l-- {
			e := (freqEval(l, nz, grid, x, y, ad) - des[l]) * wt[l]
			if nut*e-comp > 0.0 {
				comp = nut * e
				down(l, nzz)
				luck += 10

				kn := iext[nzz]
				for i := 1; i <= nfcns; i++ {
					iext[i] = iext[i+1]
				}
				iext[nz] = kn
				continue IterationLoop
			}
		}

		if luck != 6 {
			for i := 1; i <= nfcns; i++ {
				iext[nzz-i] = iext[nz-i]
			}
			iext[1] = k1
		} else if jchnge <= 0 {
			break
		}
	}

	/*
	 *    CALCULATION OF THE COEFFICIENTS OF THE BEST APPROXIMATION
	 *    USING THE INVERSE DISCRETE FOURIER TRANSFORM
	 */
	nm1 := nfcns - 1
	fsh := 1.0e-06
	gtemp := grid[1]
	x[nzz] = -2.0
	cn := float64(2*nfcns - 1)
	delf := 1.0 / cn
	l := 1
	kkk := 0

	if grid[1] < 0.01 && grid[ngrid] > 0.49 {
		kkk = 1
	}

	if nfcns <= 3 {
		kkk = 1
	}

	var aa, bb float64
	if kkk != 1 {
		dtemp := math.Cos(pi2 * grid[1])
		dnum := math.Cos(pi2 * grid[ngrid])
		aa = 2.0 / (dtemp - dnum)
		bb = -(dtemp + dnum) / (dtemp - dnum)
	}

	if tr != nil {
		tr.final(niter, kkk, luck, jchnge)
	}

	for j := 1; j <= nfcns; j++ {
		ft := float64(j-1) * delf
		xt := math.Cos(pi2 * ft)
		if kkk != 1 {
			xt = (xt - bb) / aa
			// The reference computes ATAN2(SQRT(1-XT*XT),XT), the same
			// angle, but loses about half its digits to the cancellation
			// in 1-XT*XT -- and this branch runs precisely where XT is
			// near 1 (around 0.998 when the grid starts at 0.01).
			ft = math.Acos(xt) / pi2
		}
		for {
			xe := x[l]
			if xt > xe {
				if (xt - xe) < fsh {
					a[j] = y[l]
					break
				}
				grid[1] = ft
				a[j] = freqEval(1, nz, grid, x, y, ad)
				break
			}
			if (xe - xt) < fsh {
				a[j] = y[l]
				break
			}
			l++
		}
		if l > 1 {
			l--
		}
	}

	grid[1] = gtemp
	dden := pi2 / cn
	for j := 1; j <= nfcns; j++ {
		dtemp := 0.0
		dnum := float64(j-1) * dden
		if nm1 >= 1 {
			for k := 1; k <= nm1; k++ {
				dtemp += a[k+1] * math.Cos(dnum*float64(k))
			}
		}
		alpha[j] = 2.0*dtemp + a[1]
	}

	for j := 2; j <= nfcns; j++ {
		alpha[j] *= 2.0 / cn
	}
	alpha[1] /= cn

	if kkk != 1 {
		p[1] = 2.0*alpha[nfcns]*bb + alpha[nm1]
		p[2] = 2.0 * aa * alpha[nfcns]
		q[1] = alpha[nfcns-2] - alpha[nfcns]
		for j := 2; j <= nm1; j++ {
			if j >= nm1 {
				aa *= 0.5
				bb *= 0.5
			}
			p[j+1] = 0.0
			for k := 1; k <= j; k++ {
				a[k] = p[k]
				p[k] = 2.0 * bb * a[k]
			}
			p[2] += a[1] * 2.0 * aa
			for k := 1; k <= j-1; k++ {
				p[k] += q[k] + aa*a[k+1]
			}
			for k := 3; k <= j+1; k++ {
				p[k] += aa * a[k-1]
			}

			if j != nm1 {
				for k := 1; k <= j; k++ {
					q[k] = -a[k]
				}
				q[1] += alpha[nfcns-1-j]
			}
		}
		for j := 1; j <= nfcns; j++ {
			alpha[j] = p[j]
		}
	}

	if nfcns <= 3 {
		alpha[nfcns+1] = 0.0
		alpha[nfcns+2] = 0.0
	}
	return dev, err
}

/*
 *-----------------------------------------------------------------------
 * FUNCTION: EFF
 *  FUNCTION TO CALCULATE THE DESIRED MAGNITUDE RESPONSE
 *  AS A FUNCTION OF FREQUENCY.
 *  AN ARBITRARY FUNCTION OF FREQUENCY CAN BE
 *  APPROXIMATED IF THE USER REPLACES THIS FUNCTION
 *  WITH THE APPROPRIATE CODE TO EVALUATE THE IDEAL
 *  MAGNITUDE.  NOTE THAT THE PARAMETER FREQ IS THE
 *  VALUE OF NORMALIZED FREQUENCY NEEDED FOR EVALUATION.
 *-----------------------------------------------------------------------
 */
func eff(freq float64, fx []float64, lband int, filterType FilterType) float64 {
	if filterType != Differentiator {
		return fx[lband]
	}
	return fx[lband] * freq
}

/*
 *-----------------------------------------------------------------------
 * FUNCTION: WATE
 *  FUNCTION TO CALCULATE THE WEIGHT FUNCTION AS A FUNCTION
 *  OF FREQUENCY.  SIMILAR TO THE FUNCTION EFF, THIS FUNCTION CAN
 *  BE REPLACED BY A USER-WRITTEN ROUTINE TO CALCULATE ANY
 *  DESIRED WEIGHTING FUNCTION.
 *-----------------------------------------------------------------------
 */
func wate(freq float64, fx, wtx []float64, lband int, filterType FilterType) float64 {
	if filterType != Differentiator {
		return wtx[lband]
	}
	if fx[lband] >= 0.0001 {
		return wtx[lband] / freq
	}
	return wtx[lband]
}

// Band is one band of a filter specification.
type Band struct {
	// Lower and Upper are the band edges, in normalized frequency -- cycles
	// per sample, so the Nyquist frequency is 0.5 -- unless the Spec gives a
	// SampleRate, in which case they are in the same units as that.
	Lower, Upper float64
	// Response is the desired magnitude across this band, or the desired
	// slope for a Differentiator.
	Response float64
	// Weight is how much this band matters relative to the others. The
	// ripple it ends up with is the design's deviation divided by this, so a
	// band weighted 10 gets a tenth the ripple of a band weighted 1. It must
	// be positive; only the ratios between bands affect the result.
	Weight float64
}

// Spec describes a filter to design. The zero value of each optional field
// selects the reference program's default.
type Spec struct {
	// NumTaps is the filter length, which must be at least 3.
	NumTaps int

	// Bands must increase and must neither overlap nor touch: every edge is
	// strictly greater than the one before it. The gaps between them are the
	// transition bands, which the design leaves unconstrained -- a filter
	// gets its sharpness from a narrow gap and pays for it in ripple.
	Bands []Band

	// Type selects the class of filter. The zero value is BandPass.
	Type FilterType

	// SampleRate, if non-zero, is the rate the band edges are given against,
	// so they can be written in Hz rather than normalized. Zero leaves them
	// normalized to [0, 0.5].
	SampleRate float64

	// GridDensity is how finely the frequency axis is sampled. Zero selects
	// 16. Raising it costs time in proportion and tightens how closely the
	// reported deviation bounds the true continuous response: at 16 the
	// response overshoots it by up to about 3%, at 32 by about 0.1%.
	GridDensity int

	// MaxIter caps the number of exchange iterations. Zero selects 25.
	MaxIter int
}

// Design returns the impulse response of the linear-phase FIR filter that best
// approximates spec in the weighted Chebyshev sense, along with the deviation:
// the achieved weighted error, so the ripple in a band is the deviation divided
// by that band's weight.
//
// The coefficients are symmetric for BandPass and antisymmetric otherwise, and
// an antisymmetric filter of odd length has a zero at its center.
//
// If the exchange does not converge, Design returns ErrMaxIterations or
// ErrDeviationDecreased together with the coefficients it reached. The second
// usually still describes a usable filter, which the caller can confirm from
// the deviation or with an FFT. A specification the algorithm cannot
// approximate at all -- typically a band so narrow it contributes a single
// point to the grid -- gives ErrIllConditioned and no coefficients.
//
// The deviation is worth looking at even when the error is nil: a value far
// below anything the band edges and filter length could support means the
// problem was degenerate rather than easy.
func Design(spec Spec) ([]float64, float64, error) {
	h, dev, _, err := design(spec, nil)
	return h, dev, err
}

// design is Design with the trace hook and the dense grid exposed, for tests.
func design(spec Spec, tr *trace) ([]float64, float64, []float64, error) {
	var change float64
	var ngrid int

	fail := func(msg string) ([]float64, float64, []float64, error) {
		return nil, 0, nil, errors.New("firpm: " + msg)
	}

	if spec.NumTaps < 3 {
		return fail("NumTaps must be at least 3")
	}
	// The grid arrays below are sized from NumTaps times GridDensity, and that
	// product is what has to be bounded: either factor alone can be modest
	// while the product overflows int, after which make is handed a negative
	// length and panics instead of reporting anything.
	if spec.NumTaps > maxNumTaps {
		return fail("NumTaps must be at most " + strconv.Itoa(maxNumTaps))
	}
	if spec.GridDensity > maxGridDensity {
		return fail("GridDensity must be at most " + strconv.Itoa(maxGridDensity))
	}
	if spec.Type < BandPass || spec.Type > Hilbert {
		return fail("unknown filter type")
	}
	if spec.SampleRate < 0 {
		return fail("SampleRate must not be negative")
	}
	if len(spec.Bands) == 0 {
		return fail("at least one band is required")
	}

	// Flatten to the edge/response/weight arrays the algorithm works in,
	// normalizing to cycles per sample on the way.
	scale := 1.0
	limit := "0.5"
	if spec.SampleRate != 0 {
		scale = 1.0 / spec.SampleRate
		limit = "SampleRate/2"
	}
	nbands := len(spec.Bands)
	bands := make([]float64, 0, 2*nbands)
	response := make([]float64, nbands)
	weight := make([]float64, nbands)
	for i, b := range spec.Bands {
		// Written as !(x > 0) so that a NaN fails: every comparison against one is
		// false, so a NaN weight passes "weight <= 0" and then spreads through
		// the whole exchange.
		if !(b.Weight > 0) {
			return fail("band weights must be positive and not NaN")
		}
		if math.IsNaN(b.Response) {
			return fail("band responses must not be NaN")
		}
		bands = append(bands, b.Lower*scale, b.Upper*scale)
		response[i] = b.Response
		weight[i] = b.Weight
	}
	for i, e := range bands {
		if !(e >= 0.0 && e <= 0.5) {
			return fail("band edges must lie in [0, " + limit + "] and must not be NaN")
		}
		// Strictly increasing, not merely non-decreasing: a repeated edge
		// puts the same frequency on the grid twice, so two interpolation
		// nodes coincide and the Lagrange denominator is zero. The
		// reference does not check, and returns NaN.
		if i > 0 && e <= bands[i-1] {
			return fail("band edges must increase, and bands must not touch or overlap")
		}
	}

	numtaps := spec.NumTaps
	filterType := spec.Type
	gridDensity := spec.GridDensity
	if gridDensity <= 0 {
		gridDensity = defaultGridDensity
	}
	maxiter := spec.MaxIter
	if maxiter <= 0 {
		maxiter = defaultMaxIterations
	}

	/* Set up problem on dense grid */

	neg := filterType != BandPass
	nodd := numtaps%2 == 1
	nfcns := numtaps / 2
	if nodd && !neg {
		nfcns++
	}

	// nzz, the highest index the exchange reaches.
	dimsize := nfcns + 2
	if gridDensity > maxWorkSize/dimsize {
		return fail("NumTaps times GridDensity is too large to build a grid for")
	}
	wrksize := gridDensity * dimsize
	/* Note:  code assumes these arrays start at 1 */
	h := make([]float64, numtaps+1)
	des := make([]float64, wrksize+1)
	grid := make([]float64, wrksize+1)
	wt := make([]float64, wrksize+1)
	alpha := make([]float64, dimsize+1)
	iext := make([]int, dimsize+1)

	/*
	 * SET UP THE DENSE GRID. THE NUMBER OF POINTS IN THE GRID
	 * IS (FILTER LENGTH + 1)*GRID DENSITY/2
	 */
	grid[1] = bands[0]
	delf := 0.5 / float64(gridDensity*nfcns)
	if neg {
		if bands[0] < delf {
			grid[1] = delf
		}
	}

	/*
	 * CALCULATE THE DESIRED MAGNITUDE RESPONSE AND THE WEIGHT
	 * FUNCTION ON THE GRID
	 */
	j := 1
	l := 1
	lband := 0
	for {
		fup := bands[l]
		for {
			temp := grid[j]
			des[j] = eff(temp, response, lband, filterType)
			wt[j] = wate(temp, response, weight, lband, filterType)
			j++
			if j > wrksize {
				return fail("too many points or too dense a grid")
			}
			grid[j] = temp + delf
			if grid[j] > fup {
				break
			}
		}

		grid[j-1] = fup
		des[j-1] = eff(fup, response, lband, filterType)
		wt[j-1] = wate(fup, response, weight, lband, filterType)
		lband++
		l += 2
		if lband >= nbands {
			break
		}
		grid[j] = bands[l-1]
	}

	ngrid = j - 1
	if neg == nodd {
		if grid[ngrid] > 0.5-delf {
			ngrid--
		}
	}

	grid = grid[:ngrid+1]

	/*
	 * SET UP A NEW APPROXIMATION PROBLEM WHICH IS EQUIVALENT
	 * TO THE ORIGINAL PROBLEM
	 */
	if !neg {
		if !nodd {
			for j := 1; j <= ngrid; j++ {
				change = math.Cos(pi * grid[j])
				des[j] /= change
				wt[j] *= change
			}
		}
	} else {
		if nodd {
			for j := 1; j <= ngrid; j++ {
				change = math.Sin(pi2 * grid[j])
				des[j] /= change
				wt[j] *= change
			}
		} else {
			for j := 1; j <= ngrid; j++ {
				change = math.Sin(pi * grid[j])
				des[j] /= change
				wt[j] *= change
			}
		}
	}

	/*
	 * INITIAL GUESS FOR THE EXTREMAL FREQUENCIES--EQUALLY
	 * SPACED ALONG THE GRID
	 */
	temp := float64(ngrid-1) / float64(nfcns)
	for j := 1; j <= nfcns; j++ {
		iext[j] = int(float64(j-1)*temp + 1.0)
	}
	iext[nfcns+1] = ngrid
	nm1 := nfcns - 1
	nz := nfcns + 1

	// The exchange needs nz distinct extremal frequencies, which the guess above
	// can supply only if the grid is at least that long. The reference does not
	// check, and returns NaN.
	if ngrid < nz {
		return fail("bands are too narrow for this many taps at this grid density")
	}

	dev, err := remez(des, grid, wt, ngrid, iext, alpha, nfcns, maxiter, dimsize, tr)

	/*
	 * CALCULATE THE IMPULSE RESPONSE.
	 */
	if !neg {
		if nodd {
			for j := 1; j <= nm1; j++ {
				h[j] = 0.5 * alpha[nz-j]
			}
			h[nfcns] = alpha[1]
		} else {
			h[1] = 0.25 * alpha[nfcns]
			for j := 2; j <= nm1; j++ {
				h[j] = 0.25 * (alpha[nz-j] + alpha[nfcns+2-j])
			}
			h[nfcns] = 0.5*alpha[1] + 0.25*alpha[2]
		}
	} else {
		if nodd {
			h[1] = 0.25 * alpha[nfcns]
			h[2] = 0.25 * alpha[nm1]
			for j := 3; j <= nm1; j++ {
				h[j] = 0.25 * (alpha[nz-j] - alpha[nfcns+3-j])
			}
			h[nfcns] = 0.5*alpha[1] - 0.25*alpha[3]
			h[nz] = 0.0
		} else {
			h[1] = 0.25 * alpha[nfcns]
			for j := 2; j <= nm1; j++ {
				h[j] = 0.25 * (alpha[nz-j] - alpha[nfcns+2-j])
			}
			h[nfcns] = 0.5*alpha[1] - 0.25*alpha[2]
		}
	}

	for j := 1; j <= nfcns; j++ {
		k := numtaps + 1 - j
		if !neg {
			h[k] = h[j]
		} else {
			h[k] = -h[j]
		}
	}
	if neg && nodd {
		h[nz] = 0.0
	}

	// A band narrow enough to put a single point on the grid drives the deviation
	// towards zero, and from there the coefficient recurrence overflows; the
	// reference hands the result back unremarked.
	if math.IsNaN(dev) || math.IsInf(dev, 0) {
		return nil, dev, grid, ErrIllConditioned
	}
	for _, c := range h[1:] {
		if math.IsNaN(c) || math.IsInf(c, 0) {
			return nil, dev, grid, ErrIllConditioned
		}
	}

	return h[1:], dev, grid, err
}
