package f32

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// kernelFamilies are the sets of kernels that every architecture has to
// supply, each split across mutually exclusive files matched by glob.
var kernelFamilies = []struct {
	glob  string
	names []string
}{
	// The per-width kernels behind dsp's generic vector functions.
	{"math32_*.go", []string{"Scale", "Max", "Min", "CAbs"}},
	// The FIR and dot-product leaves behind the checked wrappers in fir.go.
	{"fir_*.go", []string{"firF32", "cfirReal", "cfir", "dotF32", "cdotReal"}},
	// The per-sample kernels, assembly on arm and the references elsewhere.
	{"sample_*.go", []string{"FastAtan2", "FastAtan2Fine", "Rotate90", "FMDemod", "BoxcarDecimate", "RationalBoxcarDecimate"}},
}

// TestKernelSetComplete parses the files declaring each family of kernels,
// groups them by build tag, and requires every group to declare the family's
// whole set of names.
//
// A family lives in mutually exclusive files whose tags partition the
// architectures by hand, the last a negated tag nobody edits when adding a
// kernel to one architecture. A missing name fails only on the architecture
// nobody cross-compiles — I16ToI16LE once shipped with no stub on 386. Parsing
// checks every group from one host; linking still catches a declaration with no
// TEXT symbol behind it.
func TestKernelSetComplete(t *testing.T) {
	for _, fam := range kernelFamilies {
		t.Run(fam.glob, func(t *testing.T) {
			checkKernelFamily(t, fam.glob, fam.names)
		})
	}
}

func checkKernelFamily(t *testing.T, glob string, kernelNames []string) {
	files, err := filepath.Glob(glob)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		// The test binary is running somewhere without the package source --
		// under qemu-test, say.
		t.Skipf("no %s alongside the test binary", glob)
	}
	groups := make(map[string]map[string]bool)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		names := make(map[string]bool)
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			if slices.Contains(kernelNames, fn.Name.Name) {
				names[fn.Name.Name] = true
			}
		}
		if len(names) == 0 {
			continue
		}
		// The build tag plus the filename suffix is what actually selects a
		// file, so the two together identify the group.
		groups[buildKey(f, src)] = names
	}

	if len(groups) < 2 {
		t.Fatalf("found %d kernel groups, expected one per architecture; the glob or the file naming changed", len(groups))
	}
	for key, names := range groups {
		for _, k := range kernelNames {
			if !names[k] {
				t.Errorf("build group %q declares %v but not %s", key, sortedKeys(names), k)
			}
		}
	}
}

// buildKey identifies the constraint selecting a file: its //go:build line if
// any, else the filename, which carries the GOARCH suffix instead.
func buildKey(name string, src []byte) string {
	for line := range strings.Lines(string(src)) {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "//go:build "); ok {
			return name + " " + after
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return name
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
