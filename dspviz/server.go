package dspviz

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"strconv"
)

//go:embed web
var webFS embed.FS

// ServerOptions bounds what a request may ask for. Every field's zero value
// selects the default beside it.
//
// The limits are not decoration. The work a request costs is set entirely by
// numbers in the request, so without them a single line of JSON asking for a
// 2^28 point transform is a denial of service.
type ServerOptions struct {
	MaxPoints int     // frequency grid points; 4096
	MaxFrame  int     // measurement frame; 1 << 18
	MaxSweep  float64 // sweep seconds; 30
	MaxPixels int     // spectrogram width times height; 4 << 20

	// MaxSamples bounds points*frame, which is what the tone method actually
	// costs: one frame-long tone per grid point. Clamping the two separately
	// leaves their product three orders of magnitude above either, so a request
	// for the largest of both is a denial of service written in a URL.
	MaxSamples int // 1 << 24, which is the defaults multiplied
}

func (o ServerOptions) withDefaults() ServerOptions {
	if o.MaxPoints <= 0 {
		o.MaxPoints = 4096
	}
	if o.MaxFrame <= 0 {
		o.MaxFrame = 1 << 18
	}
	if o.MaxSweep <= 0 {
		o.MaxSweep = 30
	}
	if o.MaxPixels <= 0 {
		o.MaxPixels = 4 << 20
	}
	if o.MaxSamples <= 0 {
		o.MaxSamples = 1 << 24
	}
	return o
}

// Server serves the interactive page and the JSON behind it.
//
// It holds no filter state of its own, and must not: every request builds its
// own processor from the catalog. Nothing in dsp is safe for concurrent use and
// processors carry delay lines, so a shared filter would contaminate both
// requests.
type Server struct {
	opt ServerOptions
	mux *http.ServeMux
}

// NewServer returns a Server, which is an http.Handler.
func NewServer(opt ServerOptions) (*Server, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	s := &Server{opt: opt.withDefaults(), mux: http.NewServeMux()}

	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(sub, "index.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	})
	s.mux.HandleFunc("GET /api/filters", s.handleFilters)
	s.mux.HandleFunc("POST /api/analyze", s.handleAnalyze)
	s.mux.HandleFunc("GET /api/sweep.png", s.handleSweep)
	s.mux.HandleFunc("GET /api/chart.svg", s.handleChart)
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) handleFilters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"kinds": Catalog()})
}

// analyzeRequest is what the page posts.
type analyzeRequest struct {
	Kind    string             `json:"kind"`
	Params  map[string]float64 `json:"params"`
	Choices map[string]string  `json:"choices"`
	Points  int                `json:"points"`
	Frame   int                `json:"frame"`
	Taps    int                `json:"taps"`
	ToneHz  float64            `json:"toneHz"`
}

type curve struct {
	X      []float64 `json:"x"`
	Y      []float64 `json:"y"`
	XLabel string    `json:"xLabel"`
	YLabel string    `json:"yLabel"`
	XLog   bool      `json:"xLog"`
	Title  string    `json:"title"`
	YMin   *float64  `json:"yMin,omitempty"`
	YMax   *float64  `json:"yMax,omitempty"`
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var req analyzeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "malformed request: " + err.Error()})
		return
	}
	rep, err := s.analyze(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	db := rep.Response.MagnitudeDB()
	phase := rep.Response.UnwrappedPhase()
	for i := range phase {
		phase[i] *= 180 / math.Pi
	}
	idx := make([]float64, len(rep.Impulse))
	for i := range idx {
		idx[i] = float64(i)
	}

	curves := map[string]curve{
		"magnitude": {X: rep.Response.Freq, Y: db, XLabel: "Hz", YLabel: "dB", XLog: true,
			Title: "Magnitude", YMin: new(math.Max(-140, minOf(db)-6)), YMax: new(ceilTo(maxOf(db)+6, 6))},
		"phase": {X: rep.Response.Freq, Y: phase, XLabel: "Hz", YLabel: "degrees", XLog: true,
			Title: "Phase"},
		"groupdelay": {X: rep.Response.Freq, Y: rep.Response.GroupDelay(), XLabel: "Hz", YLabel: "samples",
			XLog: true, Title: "Group delay"},
		"impulse": {X: idx, Y: rep.Impulse, XLabel: "sample", YLabel: "amplitude",
			Title: "Impulse response"},
		"step": {X: idx, Y: rep.Step, XLabel: "sample", YLabel: "amplitude",
			Title: "Step response"},
	}
	if rep.Spectrum != nil {
		curves["tone"] = curve{X: rep.Spectrum.Freq, Y: rep.Spectrum.MagDB, XLabel: "Hz", YLabel: "dBFS",
			Title: fmt.Sprintf("Tone at %.0f Hz", rep.Spectrum.Fundamental),
			YMin:  new(math.Max(-200, minOf(rep.Spectrum.MagDB)-6)), YMax: new(6.0)}
	}

	m := rep.Metrics
	metrics := map[string]any{
		"cutoffHz":          m.CutoffHz,
		"transitionHz":      m.TransitionHz,
		"passbandRippleDB":  m.Passband.RippleDB,
		"stopbandPeakDB":    m.Stopband.MaxDB,
		"groupDelaySamples": m.GroupDelaySamples,
		"method":            rep.Response.Method.String(),
		"inRate":            rep.Rates[0],
		"outRate":           rep.Rates[1],
	}
	if rep.Spectrum != nil {
		metrics["sfdrDB"] = rep.Spectrum.SFDR()
		metrics["thdDB"] = rep.Spectrum.THD(10)
	}

	warnings := rep.Warnings
	if gw := rep.Response.GridWarning(); gw != "" {
		warnings = append(warnings, gw)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": metrics, "curves": curves, "warnings": warnings,
		"bands": rep.Bands,
	})
}

// analyze builds a fresh processor and measures it. Fresh per request, always:
// see the note on Server.
func (s *Server) analyze(req analyzeRequest) (*Report, error) {
	p, err := Build(req.Kind, req.Params, req.Choices)
	if err != nil {
		return nil, err
	}
	points := clampInt(req.Points, 64, s.opt.MaxPoints, 1024)
	frame := clampInt(req.Frame, 256, s.opt.MaxFrame, 1<<14)
	// Points is the one to give up, since the frame sets the measurement floor
	// and a coarser grid only makes the curve blockier.
	if s.opt.MaxSamples/frame < points {
		points = max(64, s.opt.MaxSamples/frame)
	}
	taps := clampInt(req.Taps, 8, 4096, 128)
	tone := req.ToneHz
	if tone <= 0 {
		tone = 997
	}
	return Analyze(p, ReportOptions{
		Points: points, Frame: frame, Taps: taps, ToneHz: tone, SweepSec: -1,
	})
}

func (s *Server) handleSweep(w http.ResponseWriter, r *http.Request) {
	req, err := requestFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p, err := Build(req.Kind, req.Params, req.Choices)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	sec := clampFloat(floatParam(q, "seconds"), 0.25, s.opt.MaxSweep, 4)
	width := clampInt(intParam(q, "w"), 64, 4096, 900)
	height := clampInt(intParam(q, "h"), 64, 2048, 320)
	if width*height > s.opt.MaxPixels {
		http.Error(w, "image too large", http.StatusBadRequest)
		return
	}

	// Twice the pixels asked for, so the reducer bounds the frames held rather
	// than the sweep length doing it: thirty seconds at a 256-sample hop is
	// five thousand columns for a 900-pixel image.
	sg, err := Sweep(p, SweepOptions{Duration: sec}, STFTOptions{
		Size:    clampInt(intParam(q, "stft"), 256, 8192, 1024),
		Columns: 2 * width,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	if err := sg.WritePNG(w, paletteOrHeat(q.Get("palette")), SpectrogramOptions{
		// A floor of zero means "choose one", so it is passed through; anything
		// else is bounded, since it reaches an axis range and a color scale.
		Width: width, Height: height, FloorDB: math.Max(-400, math.Min(0, floatParam(q, "floor"))),
	}); err != nil {
		slog.Error("writing the sweep raster", "err", err)
	}
}

func (s *Server) handleChart(w http.ResponseWriter, r *http.Request) {
	req, err := requestFromQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rep, err := s.analyze(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.URL.Query().Get("chart")
	c, ok := rep.Charts(ChartOptions{})[name]
	if !ok {
		http.Error(w, "no chart named "+strconv.Quote(name), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(req.Kind+"-"+name+".svg"))
	if err := c.WriteSVG(w); err != nil {
		slog.Error("writing a chart", "err", err)
	}
}

// requestFromQuery reads the same request the JSON endpoint takes out of a
// query string, so that a chart or a raster can be a plain link the browser can
// fetch or the user can save.
func requestFromQuery(r *http.Request) (analyzeRequest, error) {
	q := r.URL.Query()
	req := analyzeRequest{
		Kind:    q.Get("kind"),
		Params:  map[string]float64{},
		Choices: map[string]string{},
		Points:  intParam(q, "points"),
		Frame:   intParam(q, "frame"),
		Taps:    intParam(q, "taps"),
		ToneHz:  floatParam(q, "toneHz"),
	}
	k, ok := KindByName(req.Kind)
	if !ok {
		return req, errors.New("unknown filter kind " + strconv.Quote(req.Kind))
	}
	for _, p := range k.Params {
		if v := q.Get(p.Name); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return req, fmt.Errorf("%s: %w", p.Name, err)
			}
			req.Params[p.Name] = f
		}
	}
	for _, c := range k.Choices {
		if v := q.Get(c.Name); v != "" {
			req.Choices[c.Name] = v
		}
	}
	return req, nil
}

// paletteOrHeat names a palette for a request, which must not be able to fail
// over a misspelled query parameter.
func paletteOrHeat(name string) Palette {
	if p, ok := PaletteByName(name); ok {
		return p
	}
	return HeatPalette()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(sanitize(v)); err != nil {
		slog.Error("encoding a response", "err", err)
	}
}

func clampInt(v, lo, hi, def int) int {
	if v <= 0 {
		return def
	}
	return max(lo, min(v, hi))
}

func clampFloat(v, lo, hi, def float64) float64 {
	if v <= 0 {
		return def
	}
	return math.Max(lo, math.Min(v, hi))
}

func intParam(q map[string][]string, name string) int {
	v, _ := strconv.Atoi(first(q, name))
	return v
}

func floatParam(q map[string][]string, name string) float64 {
	v, _ := strconv.ParseFloat(first(q, name), 64)
	return v
}

func first(q map[string][]string, name string) string {
	if v := q[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}
