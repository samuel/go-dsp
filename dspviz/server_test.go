package dspviz

import (
	"bytes"
	"encoding/json"
	"image/png"
	"math"
	"math/cmplx"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/samuel/go-dsp/dsp"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := NewServer(ServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}

func postAnalyze(t *testing.T, srv *httptest.Server, body string) (map[string]any, int) {
	t.Helper()
	res, err := http.Post(srv.URL+"/api/analyze", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close() //nolint:errcheck // reading a test response body
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decoding the response: %v", err)
	}
	return out, res.StatusCode
}

// TestServerAnalyzeMatchesFreqz checks the wiring rather than the arithmetic:
// what comes back over HTTP has to be the same number dsp.Freqz gives for the
// same filter.
func TestServerAnalyzeMatchesFreqz(t *testing.T) {
	srv := testServer(t)
	out, code := postAnalyze(t, srv,
		`{"kind":"biquad","choices":{"type":"lowpass"},"params":{"rate":48000,"freq":1000,"q":0.7071},"points":512,"frame":4096}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, out["error"])
	}

	m := out["metrics"].(map[string]any)
	if got := m["cutoffHz"].(float64); math.Abs(got-1000) > 5 {
		t.Errorf("cutoff %v Hz, want about 1000", got)
	}
	if m["method"] != "exact" {
		t.Errorf("method %v, want exact for a filter with coefficients", m["method"])
	}

	// Compare the magnitude curve against the closed form at the cutoff.
	c := out["curves"].(map[string]any)["magnitude"].(map[string]any)
	xs := c["x"].([]any)
	ys := c["y"].([]any)
	lp := must(dsp.NewLowPassBiQuad[float64](48000, 1000, 0.7071))
	b, a := lp.Coefficients()
	for i, xv := range xs {
		f := xv.(float64)
		if f < 900 || f > 1100 {
			continue
		}
		want := 20 * math.Log10(cmplx.Abs(dsp.Freqz(b, a, []float64{2 * math.Pi * f / 48000})[0]))
		if got := ys[i].(float64); math.Abs(got-want) > 1e-9 {
			t.Fatalf("%.1f Hz: server said %v dB, Freqz says %v", f, got, want)
		}
	}
}

// TestServerBuildsAFreshFilterPerRequest is the test for the mistake this
// design is most likely to make. Nothing in dsp is safe for concurrent use, and
// the processors carry delay lines, so a filter shared between requests would
// contaminate both -- intermittently, which is the worst way to find out. Two
// different filters measured at once must give two different, correct answers.
func TestServerBuildsAFreshFilterPerRequest(t *testing.T) {
	srv := testServer(t)
	bodies := []struct {
		body   string
		cutoff float64
	}{
		{`{"kind":"biquad","choices":{"type":"lowpass"},"params":{"rate":48000,"freq":500,"q":0.7071},"points":512,"frame":4096}`, 500},
		{`{"kind":"biquad","choices":{"type":"lowpass"},"params":{"rate":48000,"freq":5000,"q":0.7071},"points":512,"frame":4096}`, 5000},
	}

	var wg sync.WaitGroup
	errs := make([]error, 0)
	var mu sync.Mutex
	for range 8 {
		for _, c := range bodies {
			wg.Go(func() {
				out, code := postAnalyze(t, srv, c.body)
				mu.Lock()
				defer mu.Unlock()
				if code != http.StatusOK {
					errs = append(errs, nil)
					t.Errorf("status %d", code)
					return
				}
				got := out["metrics"].(map[string]any)["cutoffHz"].(float64)
				if math.Abs(got-c.cutoff) > c.cutoff*0.02 {
					t.Errorf("cutoff %v, want about %v: the two requests are sharing a filter", got, c.cutoff)
				}
			})
		}
	}
	wg.Wait()
}

// TestServerClampsSizes checks that the numbers which set how much work a
// request costs are bounded. Without this an unclamped transform length is a
// one-line denial of service.
func TestServerClampsSizes(t *testing.T) {
	srv := testServer(t)
	out, code := postAnalyze(t, srv,
		`{"kind":"biquad","params":{"rate":48000,"freq":1000},"points":100000000,"frame":1073741824,"taps":100000000}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, out["error"])
	}
	c := out["curves"].(map[string]any)["magnitude"].(map[string]any)
	if n := len(c["x"].([]any)); n > 4096 {
		t.Errorf("asked for 1e8 grid points and got %d; the clamp is not working", n)
	}
	imp := out["curves"].(map[string]any)["impulse"].(map[string]any)
	if n := len(imp["x"].([]any)); n > 4096 {
		t.Errorf("asked for 1e8 impulse samples and got %d", n)
	}
}

func TestServerRejectsBadRequests(t *testing.T) {
	srv := testServer(t)
	if out, code := postAnalyze(t, srv, `{"kind":"nonesuch"}`); code != http.StatusBadRequest {
		t.Errorf("an unknown filter gave %d: %v", code, out)
	}
	if out, code := postAnalyze(t, srv, `not json`); code != http.StatusBadRequest {
		t.Errorf("malformed JSON gave %d: %v", code, out)
	}

	res, err := http.Get(srv.URL + "/api/sweep.png?kind=biquad&w=100000&h=100000")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close() //nolint:errcheck // reading a test response body
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("an enormous raster gave %d, want 400", res.StatusCode)
	}
}

// TestServerNotchSurvivesJSON checks the case that would otherwise take a whole
// response down: a notch filter has a true zero in its magnitude, which is
// negative infinity in decibels, and encoding/json refuses to encode that.
func TestServerNotchSurvivesJSON(t *testing.T) {
	srv := testServer(t)
	out, code := postAnalyze(t, srv,
		`{"kind":"biquad","choices":{"type":"notch"},"params":{"rate":48000,"freq":1000,"q":8},"points":512,"frame":4096}`)
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, out["error"])
	}
	ys := out["curves"].(map[string]any)["magnitude"].(map[string]any)["y"].([]any)
	var deepest float64
	for _, v := range ys {
		deepest = math.Min(deepest, v.(float64))
	}
	if deepest > -18 {
		t.Errorf("the deepest point of a notch is %v dB, which is not a notch", deepest)
	}
	for _, v := range ys {
		if math.IsNaN(v.(float64)) {
			t.Error("a NaN reached the response")
		}
	}
}

// TestSanitizeHandlesInfinities is the other half: a grid that lands exactly on
// a notch gives a magnitude of zero, which is negative infinity in decibels,
// and encoding/json refuses to encode that at all -- one such point would
// otherwise take the whole response down rather than leaving a gap in one
// curve. The infinities are mapped to the edge of the plot rather than to zero,
// which would draw a spike where there is silence.
func TestSanitizeHandlesInfinities(t *testing.T) {
	in := map[string]any{
		"curves": map[string]curve{
			"m": {X: []float64{1, 2, 3}, Y: []float64{math.Inf(-1), math.NaN(), math.Inf(1)}},
		},
		"metrics": map[string]any{"a": math.Inf(-1)},
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(sanitize(in)); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	ys := out["curves"].(map[string]any)["m"].(map[string]any)["y"].([]any)
	if ys[0].(float64) >= 0 {
		t.Errorf("negative infinity became %v, want a large negative number", ys[0])
	}
	if ys[1].(float64) != 0 {
		t.Errorf("NaN became %v, want 0", ys[1])
	}
	if ys[2].(float64) <= 0 {
		t.Errorf("positive infinity became %v, want a large positive number", ys[2])
	}
}

func TestServerServesAssetsAndCharts(t *testing.T) {
	srv := testServer(t)
	for _, c := range []struct{ path, contentType string }{
		{"/", "text/html"},
		{"/static/app.js", "text/javascript"},
		{"/static/app.css", "text/css"},
		{"/api/filters", "application/json"},
		{"/api/chart.svg?kind=biquad&type=lowpass&rate=48000&freq=1000&q=0.7071&chart=magnitude", "image/svg+xml"},
		{"/api/sweep.png?kind=biquad&type=lowpass&rate=48000&freq=1000&q=0.7071&seconds=1&w=200&h=100", "image/png"},
	} {
		res, err := http.Get(srv.URL + c.path)
		if err != nil {
			t.Fatal(err)
		}
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", c.path, res.StatusCode)
			continue
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, c.contentType) {
			t.Errorf("%s: content type %q, want %q", c.path, ct, c.contentType)
		}
		if body.Len() == 0 {
			t.Errorf("%s: empty body", c.path)
		}
		if c.contentType == "image/png" {
			if _, err := png.Decode(bytes.NewReader(body.Bytes())); err != nil {
				t.Errorf("%s: the PNG does not decode: %v", c.path, err)
			}
		}
	}

	res, err := http.Get(srv.URL + "/api/chart.svg?kind=biquad&chart=nonesuch")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown chart gave %d, want 404", res.StatusCode)
	}
}

// TestCatalogBuildsEverything checks that every kind the catalog advertises can
// actually be built at its defaults, since the page offers all of them.
func TestCatalogBuildsEverything(t *testing.T) {
	for _, k := range Catalog() {
		t.Run(k.Name, func(t *testing.T) {
			params, choices := k.Defaults()
			p, err := Build(k.Name, params, choices)
			if err != nil {
				t.Fatalf("building at defaults: %v", err)
			}
			if in, out := p.Rates(); in <= 0 || out <= 0 {
				t.Fatalf("rates %v, %v", in, out)
			}
			// And every choice, since the page offers those too.
			for _, c := range k.Choices {
				for _, o := range c.Options {
					choices[c.Name] = o
					if _, err := Build(k.Name, params, choices); err != nil {
						t.Errorf("%s=%s: %v", c.Name, o, err)
					}
				}
				choices[c.Name] = c.Default
			}
			if _, err := Analyze(p, ReportOptions{Points: 128, Frame: 2048, SweepSec: -1}); err != nil {
				t.Errorf("analyzing: %v", err)
			}
		})
	}
	if _, err := Build("nonesuch", nil, nil); err == nil {
		t.Error("an unknown kind should be an error")
	}
}

// TestCatalogBuildsEveryParamExtreme sweeps every parameter of every kind to
// its advertised minimum, default and maximum, and does the whole sweep again
// at each extreme of the sample rate. That second axis is the one that matters:
// the catalog's frequencies are absolute, so the default stopband edge sits
// above Nyquist at the lowest rate offered -- which is how "firpm -rate 8000"
// came to fail to build at all rather than designing a filter for 8 kHz.
//
// A slider can be moved to either end, so anything the catalog advertises has
// to build. Failing to build is the failure; a clamped value is not, since Build
// promises to clamp rather than to reject.
func TestCatalogBuildsEveryParamExtreme(t *testing.T) {
	// The rate parameter is called "fast" on the rational decimator, which is
	// the pair of rates it resamples between.
	rateNames := map[string]string{"rational": "fast"}

	for _, k := range Catalog() {
		t.Run(k.Name, func(t *testing.T) {
			rateName := "rate"
			if n, ok := rateNames[k.Name]; ok {
				rateName = n
			}
			var rateParam Param
			for _, p := range k.Params {
				if p.Name == rateName {
					rateParam = p
				}
			}
			if rateParam.Name == "" {
				t.Fatalf("no %q parameter to sweep against", rateName)
			}

			for _, rate := range []float64{rateParam.Min, rateParam.Default, rateParam.Max} {
				for _, p := range k.Params {
					for _, label := range []string{"min", "default", "max"} {
						v := map[string]float64{"min": p.Min, "default": p.Default, "max": p.Max}[label]
						params, choices := k.Defaults()
						params[rateName] = rate
						params[p.Name] = v

						proc, err := Build(k.Name, params, choices)
						if err != nil {
							t.Errorf("%s at %s (%g) with %s %g: %v", p.Name, label, v, rateName, rate, err)
							continue
						}
						in, out := proc.Rates()
						if in <= 0 || out <= 0 || math.IsNaN(in) || math.IsNaN(out) {
							t.Errorf("%s at %s (%g) with %s %g: rates %v, %v",
								p.Name, label, v, rateName, rate, in, out)
							continue
						}
						// And the filter it built has to be measurable, since
						// building one that produces NaN is no better than not
						// building it.
						// A short frame and a coarse grid: this sweep is
						// checking that every advertised value builds
						// something measurable, not measuring it well.
						r, err := FreqResponse(proc, LinearGrid(in/64, in/2*0.99, 12),
							ResponseOptions{Frame: 2048})
						if err != nil {
							t.Errorf("%s at %s (%g) with %s %g: measuring: %v",
								p.Name, label, v, rateName, rate, err)
							continue
						}
						for i, db := range r.MagnitudeDB() {
							if math.IsNaN(db) {
								t.Errorf("%s at %s (%g) with %s %g: %.0f Hz is NaN",
									p.Name, label, v, rateName, rate, r.Freq[i])
								break
							}
						}
					}
				}
			}
		})
	}
}

// TestBandsMatchWhatFirpmDesigned checks that the bands the report labels are
// the bands the filter was designed for. Two similar calculations used to
// agree only at the defaults, and a band edge outside the band understates a
// ripple without looking wrong.
func TestBandsMatchWhatFirpmDesigned(t *testing.T) {
	if _, _, ok := Bands("biquad", nil); ok {
		t.Error("Bands claims to know a biquad's bands")
	}
	if _, _, ok := Bands("nonesuch", nil); ok {
		t.Error("Bands claims to know an unknown kind's bands")
	}

	for _, rate := range []float64{8000, 48000, 192000} {
		params, choices := KindOrFail(t, "firpm").Defaults()
		params["rate"] = rate
		pass, stop, ok := Bands("firpm", params)
		if !ok {
			t.Fatalf("rate %g: Bands reported nothing", rate)
		}
		if pass[0] != 0 || pass[1] <= 0 || stop[0] <= pass[1] || stop[1] != rate/2 {
			t.Fatalf("rate %g: pass %v stop %v", rate, pass, stop)
		}

		// The filter really does pass the one band and stop the other, which is
		// what makes these the right edges to measure against.
		p, err := Build("firpm", params, choices)
		if err != nil {
			t.Fatalf("rate %g: %v", rate, err)
		}
		r, err := FreqResponse(p, LinearGrid(pass[1]*0.02, pass[1]*0.98, 32), ResponseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i, db := range r.MagnitudeDB() {
			if db < -3 {
				t.Errorf("rate %g: the passband is %v dB at %.0f Hz", rate, db, r.Freq[i])
				break
			}
		}
		r, err = FreqResponse(p, LinearGrid(stop[0]*1.02, stop[1]*0.98, 32), ResponseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i, db := range r.MagnitudeDB() {
			if db > -20 {
				t.Errorf("rate %g: the stopband is %v dB at %.0f Hz", rate, db, r.Freq[i])
				break
			}
		}
	}
}

// KindOrFail is KindByName for a name the test knows exists.
func KindOrFail(t *testing.T, name string) Kind {
	t.Helper()
	k, ok := KindByName(name)
	if !ok {
		t.Fatalf("no catalog entry %q", name)
	}
	return k
}

// TestBuildClampsOutOfRange checks that a parameter outside the range the
// catalog advertises is brought back into it rather than building something
// nonsensical -- which matters because these values arrive over the network.
func TestBuildClampsOutOfRange(t *testing.T) {
	p, err := Build("biquad", map[string]float64{
		"rate": 48000, "freq": 1e9, "q": -5,
	}, map[string]string{"type": "lowpass"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := FreqResponse(p, LogGrid(20, 20000, 64), ResponseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range r.MagnitudeDB() {
		if math.IsNaN(v) {
			t.Fatalf("%.0f Hz is NaN: an out-of-range parameter reached the filter", r.Freq[i])
		}
	}
	// An unknown choice falls back to the default rather than building nothing.
	if _, err := Build("biquad", nil, map[string]string{"type": "nonesuch"}); err != nil {
		t.Errorf("an unknown choice should fall back, not fail: %v", err)
	}
}
