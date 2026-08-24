package firpm

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// goldenCase is one case from testdata/golden, which is the tagged state dump
// of the Fortran in testdata/remez.f. See testdata/generate.sh for how the two
// precisions are built.
type goldenCase struct {
	name string

	// The input deck, echoed back by the Fortran so the Go side drives the
	// same problem rather than a hand-copied restatement of it.
	numtaps     int
	filterType  FilterType
	gridDensity int
	bands       []float64
	response    []float64
	weight      []float64

	// Grid setup.
	neg   int
	nodd  int
	nfcns int
	ngrid int
	delf  float64

	// The exchange, iteration by iteration. luck and jchnge on an iters
	// entry are the preceding iteration's, matching the reference dump.
	iext0 []int
	iters []goldenIter

	// Final state.
	niter, kkk, luck, jchnge int
	aa, bb                   float64
	iextFinal                []int
	dev                      float64
	h                        []float64
	extremal                 []float64
	fail                     int
}

type goldenIter struct {
	niter, luck, jchnge int
	iext                []int
	dev                 float64
}

func parseGolden(t *testing.T, path string) *goldenCase {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only file, nothing to report

	g := &goldenCase{name: strings.TrimSuffix(filepath.Base(path), ".txt")}

	ints := func(fs []string) []int {
		out := make([]int, len(fs))
		for i, s := range fs {
			v, err := strconv.Atoi(s)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			out[i] = v
		}
		return out
	}
	floats := func(fs []string) []float64 {
		out := make([]float64, len(fs))
		for i, s := range fs {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			out[i] = v
		}
		return out
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) == 0 {
			continue
		}
		tag, rest := fs[0], fs[1:]
		switch tag {
		case "#IN":
			v := ints(rest)
			g.numtaps, g.gridDensity = v[0], v[3]
			// JTYPE is 1-based in the Fortran.
			g.filterType = FilterType(v[1] - 1)
		case "#EDGE":
			g.bands = floats(rest)
		case "#FX":
			g.response = floats(rest)
		case "#WTX":
			g.weight = floats(rest)
		case "#GRID":
			v := ints(rest)
			g.neg, g.nodd, g.nfcns, g.ngrid = v[0], v[1], v[2], v[3]
		case "#DELF":
			g.delf = floats(rest)[0]
		case "#IEXT0":
			g.iext0 = ints(rest)
		case "#ITER":
			v := ints(rest)
			g.iters = append(g.iters, goldenIter{niter: v[0], luck: v[1], jchnge: v[2], iext: v[3:]})
		case "#DEVIT":
			n, _ := strconv.Atoi(rest[0])
			for i := range g.iters {
				if g.iters[i].niter == n {
					g.iters[i].dev = floats(rest[1:])[0]
				}
			}
		case "#STATE":
			v := ints(rest)
			g.niter, g.kkk, g.luck, g.jchnge, g.fail = v[0], v[1], v[2], v[3], v[4]
		case "#AABB":
			v := floats(rest)
			g.aa, g.bb = v[0], v[1]
		case "#IEXTF":
			g.iextFinal = ints(rest)
		case "#H":
			g.h = append(g.h, floats(rest[1:])[0])
		case "#DEV":
			g.dev = floats(rest)[0]
		case "#EXT":
			g.extremal = floats(rest)
		case "#END":
			g.fail = ints(rest)[0]
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if g.numtaps == 0 || len(g.h) == 0 {
		t.Fatalf("%s: incomplete golden file", path)
	}
	return g
}

func loadGoldens(t *testing.T, prec string) []*goldenCase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "golden", prec, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no golden files under testdata/golden/%s; run go generate ./firpm", prec)
	}
	cases := make([]*goldenCase, len(paths))
	for i, p := range paths {
		cases[i] = parseGolden(t, p)
	}
	return cases
}

// run drives the Go implementation over the same problem the golden describes,
// collecting the trace so the trajectory can be compared step by step.
func (g *goldenCase) run(t *testing.T) (h []float64, dev float64, got *goldenCase, err error) {
	t.Helper()
	got = &goldenCase{name: g.name}
	tr := &trace{
		iteration: func(niter, luck, jchnge int, iext []int) {
			got.iters = append(got.iters, goldenIter{
				niter: niter, luck: luck, jchnge: jchnge,
				iext: append([]int(nil), iext...),
			})
		},
		deviation: func(niter int, dev float64) {
			for i := range got.iters {
				if got.iters[i].niter == niter {
					got.iters[i].dev = dev
				}
			}
		},
		final: func(niter, kkk, luck, jchnge int) {
			got.niter, got.kkk, got.luck, got.jchnge = niter, kkk, luck, jchnge
		},
	}
	var grid []float64
	h, dev, grid, err = design(g.spec(), tr)
	got.numtaps, got.ngrid = g.numtaps, len(grid)-1
	return h, dev, got, err
}

// spec rebuilds the Spec from the deck the reference echoed back, so the Go
// side drives the same problem rather than a hand-copied restatement of it.
func (g *goldenCase) spec() Spec {
	bands := make([]Band, len(g.response))
	for i := range bands {
		bands[i] = Band{
			Lower:    g.bands[2*i],
			Upper:    g.bands[2*i+1],
			Response: g.response[i],
			Weight:   g.weight[i],
		}
	}
	return Spec{
		NumTaps:     g.numtaps,
		Bands:       bands,
		Type:        g.filterType,
		GridDensity: g.gridDensity,
	}
}

// wantErr is the error the reference's IFAIL code corresponds to.
func (g *goldenCase) wantErr() error {
	switch g.fail {
	case 1:
		return ErrMaxIterations
	case 2:
		return ErrDeviationDecreased
	}
	return nil
}

// peak is the largest coefficient magnitude, which the impulse response
// comparisons are taken relative to. Individual coefficients pass through
// zero, so a per-coefficient relative error is not a meaningful scale.
func (g *goldenCase) peak() float64 {
	var p float64
	for _, c := range g.h {
		if a := math.Abs(c); a > p {
			p = a
		}
	}
	return p
}
