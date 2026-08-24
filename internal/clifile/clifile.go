// Package clifile writes a command's output to a file or to standard output.
package clifile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Write hands a writer for path to fn.
//
// "-" or "" writes to standard output, left open. Anything else is written to
// path+".tmp" and renamed over path once fn and Close succeed, so a failed run
// leaves the previous output intact. Close's error is joined to fn's, since
// only Close reports a short write on the last block.
func Write(path string, fn func(io.Writer) error) error {
	if path == "" || path == "-" {
		return fn(os.Stdout)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := errors.Join(fn(f), f.Close()); err != nil {
		// Best effort; the write already failed.
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Ext returns path's lower-case extension without the dot, and "" for standard
// output, which has no name to take one from.
func Ext(path string) string {
	if path == "" || path == "-" {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
}

// CheckExt rejects path unless its extension is one of want, or it is standard
// output. It is what turns "-o plot.png" on a line chart into a clean message.
func CheckExt(path string, want ...string) error {
	ext := Ext(path)
	if ext == "" {
		return nil
	}
	if slices.Contains(want, ext) {
		return nil
	}
	return fmt.Errorf("cannot write %s as .%s; this one is written as %s",
		filepath.Base(path), ext, strings.Join(want, " or "))
}
