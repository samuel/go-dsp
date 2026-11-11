// Package kernelset checks that every build-tag group of a kernel family
// declares the family's whole set of kernels, with the same signatures.
//
// A family lives in mutually exclusive files whose build tags partition the
// architectures by hand, the last a negated tag nobody edits when adding a
// kernel to one architecture. A missing name fails only on the architecture
// nobody cross-compiles. Parsing checks every group from one host; linking
// still catches a body-less declaration with no TEXT symbol behind it.
package kernelset

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Check parses the non-test files matching glob, groups them by build
// constraint, and requires every group that declares any of names to declare
// all of them, each with the same signature as in every other group. It skips
// when no file matches, as in a test binary running without its package
// source.
func Check(t *testing.T, glob string, names []string) {
	t.Helper()
	files, err := filepath.Glob(glob)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skipf("no %s alongside the test binary", glob)
	}

	fset := token.NewFileSet()
	groups := make(map[string]map[string]string) // group -> name -> signature
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		sigs := make(map[string]string)
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !slices.Contains(names, fn.Name.Name) {
				continue
			}
			sigs[fn.Name.Name] = signature(t, fset, fn.Type)
		}
		if len(sigs) == 0 {
			continue
		}
		groups[buildKey(f, src)] = sigs
	}

	if len(groups) < 2 {
		t.Fatalf("found %d kernel groups for %s, expected one per architecture; the glob or the file naming changed", len(groups), glob)
	}
	want := make(map[string]string)
	wantFrom := make(map[string]string)
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		sigs := groups[key]
		for _, k := range names {
			sig, ok := sigs[k]
			if !ok {
				t.Errorf("build group %q declares %v but not %s", key, slices.Sorted(maps.Keys(sigs)), k)
				continue
			}
			if w, ok := want[k]; !ok {
				want[k], wantFrom[k] = sig, key
			} else if sig != w {
				t.Errorf("%s is %s in %q but %s in %q", k, sig, key, w, wantFrom[k])
			}
		}
	}
}

// signature prints a function type with its parameter names dropped, so that
// two declarations compare equal exactly when their types do.
func signature(t *testing.T, fset *token.FileSet, ft *ast.FuncType) string {
	t.Helper()
	var b bytes.Buffer
	list := func(fl *ast.FieldList) {
		b.WriteByte('(')
		if fl != nil {
			first := true
			for _, f := range fl.List {
				for range max(len(f.Names), 1) {
					if !first {
						b.WriteString(", ")
					}
					first = false
					if err := printer.Fprint(&b, fset, f.Type); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		b.WriteByte(')')
	}
	b.WriteString("func")
	list(ft.Params)
	if ft.Results != nil {
		b.WriteByte(' ')
		list(ft.Results)
	}
	return b.String()
}

// buildKey identifies the constraint selecting a file: its //go:build line if
// any, with the filename, which carries the GOARCH suffix.
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
