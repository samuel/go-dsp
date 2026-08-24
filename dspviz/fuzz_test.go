package dspviz

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FuzzWriteSVG feeds arbitrary text into every place a caller's string reaches
// the document -- the title, the subtitle, the axis labels, the series names
// and the notes -- and requires the result to still parse.
//
// It is the moving half of what TestSVGEscapes pins by hand, and it is paired
// with the encoding/xml parse for the reason the golden helper gives: a
// document with an escaping bug in it still looks like an SVG.
func FuzzWriteSVG(f *testing.F) {
	f.Add("plain", "text", 1.0)
	f.Add(`a & b <script>alert("x")</script>`, "über <100 dB", -1e300)
	f.Add("]]>", "\x00\x01�", math.NaN())
	f.Add("&amp;", "'\"<>", math.Inf(-1))

	f.Fuzz(func(t *testing.T, title, label string, y float64) {
		c := &Chart{
			Title:    title,
			Subtitle: label,
			X:        Axis{Label: label, Min: 0, Max: 10},
			Y:        Axis{Label: title, Min: -1, Max: 1},
			Series: []Series{{
				Name: label,
				X:    []float64{0, 5, 10},
				Y:    []float64{y, 0, -y},
			}},
			Notes: []string{title, label},
		}
		var buf bytes.Buffer
		if err := c.WriteSVG(&buf); err != nil {
			t.Fatalf("WriteSVG: %v", err)
		}
		if err := parseXML(buf.Bytes()); err != nil {
			t.Fatalf("not well-formed XML: %v", err)
		}
		// Well-formed is not enough: injected text that happens to be a
		// balanced element parses perfectly and is still markup. So every
		// element in the document has to be one this package emits.
		names, err := elementNames(buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if !chartElements[name] {
				t.Fatalf("a %q element reached the document; the chart emits none", name)
			}
		}
	})
}

// chartElements is every element svg.go writes. Anything else in the output
// came out of a caller's string.
var chartElements = map[string]bool{
	"svg": true, "rect": true, "style": true, "text": true,
	"clipPath": true, "path": true, "g": true, "image": true,
}

// elementNames returns every element name in an XML document, in order.
func elementNames(b []byte) ([]string, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	var out []string
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if start, ok := tok.(xml.StartElement); ok {
			out = append(out, start.Name.Local)
		}
	}
}

// FuzzHandleAnalyze posts arbitrary bodies to the one handler that takes a
// document rather than a query string.
//
// Every number in that document decides how much work the request costs, so
// the handler clamps all of them; this checks the other half, that no
// combination of them panics or produces a response the front end cannot parse.
// A 500 is a failure here: the handler answers with 200 or with 400 and a
// message, and nothing in between.
func FuzzHandleAnalyze(f *testing.F) {
	f.Add(`{"kind":"biquad","choices":{"type":"lowpass"},"params":{"rate":48000,"freq":1000,"q":0.7071}}`)
	f.Add(`{"kind":"firpm","params":{"rate":8000,"taps":401},"points":100000,"frame":100000}`)
	f.Add(`{"kind":"rational","params":{"fast":8000,"slow":8000}}`)
	f.Add(`{"kind":"decimate","params":{"factor":-1e300},"points":-5,"frame":-5,"taps":-5,"toneHz":-1}`)
	f.Add(`{"kind":"dcblock","params":{"pole":1e308},"toneHz":1e308}`)
	f.Add(`{}`)
	f.Add(`not json`)

	s, err := NewServer(ServerOptions{MaxPoints: 256, MaxFrame: 4096, MaxSamples: 1 << 20})
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, body string) {
		req := httptest.NewRequest(http.MethodPost, "/api/analyze", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, req)

		res := w.Result()
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status %d for %q", res.StatusCode, body)
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("the response does not parse: %v (%q)", err, w.Body.String())
		}
		if res.StatusCode == http.StatusBadRequest {
			if _, ok := out["error"].(string); !ok {
				t.Fatalf("a 400 carried no error message: %q", w.Body.String())
			}
			return
		}
		// A success has to carry the three sections the page reads, and every
		// number in them has to be one JSON can express -- which is the whole
		// reason sanitize exists.
		for _, key := range []string{"metrics", "curves"} {
			if _, ok := out[key]; !ok {
				t.Fatalf("a 200 is missing %q: %q", key, w.Body.String())
			}
		}
		if strings.ContainsAny(w.Body.String(), "\x00") {
			t.Fatal("the response holds a NUL")
		}
	})
}
