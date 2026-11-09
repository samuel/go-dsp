package f32

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kernelNames are the per-width kernels the generic entry points in vector.go
// dispatch to. Every architecture has to supply all of them.
var kernelNames = []string{"Scale", "Max", "Min", "CAbs"}

// TestKernelSetComplete parses the files declaring the float32 kernels, groups
// them by build tag, and requires every group to declare the same set of names.
//
// The kernels live in six mutually exclusive files whose tags partition the
// architectures by hand, the last a negated tag nobody edits when adding a
// kernel to one architecture. A missing name fails only on the architecture
// nobody cross-compiles — I16ToI16LE once shipped with no stub on 386. Parsing
// checks every group from one host; linking still catches a declaration with no
// TEXT symbol behind it.
func TestKernelSetComplete(t *testing.T) {
	files, err := filepath.Glob("math32_*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		// The test binary is running somewhere without the package source --
		// under qemu-test, say. asm_stubs_test.go skips for the same reason.
		t.Skip("no math32_*.go alongside the test binary")
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
			for _, k := range kernelNames {
				if fn.Name.Name == k {
					names[k] = true
				}
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
	var out []string
	for _, k := range kernelNames {
		if m[k] {
			out = append(out, k)
		}
	}
	return out
}
