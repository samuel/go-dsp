package plot

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
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
