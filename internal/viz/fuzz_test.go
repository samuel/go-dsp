package viz

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
