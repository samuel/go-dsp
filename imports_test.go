package dsp

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoImportCycleWithCodec keeps the root package free of the packages
// that import it for its Sample constraint, pcm and sampleio. Go rejects the
// cycle at build time, but only once both halves exist; this names the rule.
func TestNoImportCycleWithCodec(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no package source alongside the test binary")
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			switch path {
			case "samuelks.com/dsp/pcm", "samuelks.com/dsp/sampleio":
				t.Errorf("%s imports %s, which imports dsp", f, path)
			}
		}
	}
}
