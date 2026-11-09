package dsp

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestAsmStubsAndFallbackAgree parses the two files that declare the assembly
// entry points and checks that they declare the same set with the same
// signatures.
//
// Without this, adding an entry point to asm_stubs.go and forgetting
// asm_fallback.go is a build failure only for an architecture with no assembly
// -- riscv64, ppc64le, s390x, wasm -- so it goes unnoticed until someone
// cross-compiles for one. The files are read from disk rather than reflected
// over because the two are mutually exclusive: only one of them is ever
// compiled into this test binary.
func TestAsmStubsAndFallbackAgree(t *testing.T) {
	stubs := asmEntryPoints(t, "asm_stubs.go")
	fallbacks := asmEntryPoints(t, "asm_fallback.go")

	if len(stubs) == 0 {
		t.Fatal("asm_stubs.go declared no entry points; the parse must be wrong")
	}
	for name, sig := range stubs {
		other, ok := fallbacks[name]
		if !ok {
			t.Errorf("%s is in asm_stubs.go but not in asm_fallback.go", name)
			continue
		}
		if sig != other {
			t.Errorf("%s: asm_stubs.go has %s, asm_fallback.go has %s", name, sig, other)
		}
	}
	for name := range fallbacks {
		if _, ok := stubs[name]; !ok {
			t.Errorf("%s is in asm_fallback.go but not in asm_stubs.go", name)
		}
	}
}

// asmEntryPoints returns the top-level function signatures in path, keyed by
// name and rendered back to source so the two files can be compared as text.
func asmEntryPoints(t *testing.T, path string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Skipf("Could not found asm entry point path %s", path)
		}
		t.Fatal(err)
	}
	out := make(map[string]string)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}
		var sb strings.Builder
		if err := printer.Fprint(&sb, fset, fn.Type); err != nil {
			t.Fatal(err)
		}
		out[fn.Name.Name] = sb.String()
	}
	return out
}
