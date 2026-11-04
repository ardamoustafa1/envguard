// Package testutil holds helpers shared by tests. It must only be imported
// from _test.go files.
package testutil

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update is set by `go test ./internal/<pkg> -update` and makes Golden rewrite
// golden files instead of comparing against them. Being a test-only flag, it
// is the one accepted exception to "no global mutable state".
var update = flag.Bool("update", false, "rewrite golden files")

// Golden compares got with the golden file at path, or rewrites the file when
// -update is set. Line endings of the golden file are normalized to "\n" so
// checkouts with CRLF conversion still compare equal.
func Golden(t testing.TB, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (run with -update to create it): %v", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(want, got) {
		t.Errorf("output differs from %s (run with -update to accept):\n%s", path, firstDiff(string(want), string(got)))
	}
}

func firstDiff(want, got string) string {
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d:\n  want: %s\n  got:  %s", i+1, w, g)
		}
	}
	return "(no line difference; check trailing bytes)"
}

// Updating reports whether -update was given, for tests that regenerate
// files other than plain goldens (e.g. a generated section of README.md).
func Updating() bool { return *update }
