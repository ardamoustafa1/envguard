package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestNoHiddenCharactersInSources fails on invisible or bidirectional
// control characters in source and documentation files ("Trojan Source"):
// they can make code read differently from how it compiles. Write them as
// escapes (\u202e) instead. Test data under testdata/ is exempt on purpose.
func TestNoHiddenCharactersInSources(t *testing.T) {
	skip := map[string]bool{"testdata": true, "node_modules": true, "dist": true, "bin": true, ".git": true}
	checked := 0
	err := filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(p) {
		case ".go", ".md", ".yml", ".yaml", ".sh", ".mjs", ".js", ".json", ".tape", ".txtar":
		default:
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // test
		if err != nil {
			return err
		}
		checked++
		line := 1
		for _, r := range strings.TrimPrefix(string(data), "\ufeff") {
			switch {
			case r == '\n':
				line++
			case r == '\t' || r == '\r':
			case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029':
				rel, _ := filepath.Rel(repoRoot, p)
				t.Errorf("%s:%d contains U+%04X; write it as an escape", filepath.ToSlash(rel), line, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Fatalf("only %d files checked", checked)
	}
}
