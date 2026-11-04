package main

import (
	"bytes"
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// treeDigest hashes every file (path, mode and content) under dir, .git
// included, so any change a command makes to a repository shows up.
func treeDigest(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		h.Write([]byte(rel + "\x00" + info.Mode().String() + "\x00"))
		if !d.IsDir() {
			data, err := os.ReadFile(p) //nolint:gosec // test
			if err != nil {
				return err
			}
			h.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(h.Sum(nil))
}

// TestCommandsDoNotModifyTheRepository backs the "read-only" promise: every
// analysis command leaves the analyzed tree byte-for-byte unchanged (the
// documented exceptions are init without --dry-run, --output and the GitHub
// job summary, which write only where the user pointed them).
func TestCommandsDoNotModifyTheRepository(t *testing.T) {
	demo := demoRepo(t)
	t.Setenv("PORT", "8080")
	before := treeDigest(t, demo)
	for _, args := range [][]string{
		{"check"}, {"check", "--fail-on", "never", "--format", "json"}, {"check", "--format", "sarif"},
		{"check", "--format", "github"}, {"check", "--process-env", "--env", "production"},
		{"doctor"}, {"diff"}, {"diff", "staging", "production"}, {"explain", "EG001"}, {"explain", "--list"},
		{"init", "--dry-run", "--example"}, {"--verbose", "check"}, {"version"},
	} {
		var stdout, stderr bytes.Buffer
		run(append([]string{"--root", demo}, args...), &stdout, &stderr)
		if got := treeDigest(t, demo); got != before {
			t.Fatalf("envguard %s modified the repository", strings.Join(args, " "))
		}
	}
}

// writeAPIs are the calls that create, change or delete files.
var writeAPIs = regexp.MustCompile(`\bos\.(WriteFile|Create|CreateTemp|OpenFile|Remove|RemoveAll|Rename|Mkdir|MkdirAll|Truncate|Chmod|Chtimes|Symlink|Link)\(|\bioutil\.WriteFile\(`)

// TestOnlyDocumentedFilesWriteToDisk lists the only production files allowed
// to write: init (`init.go`) and the report destinations (`output.go`:
// --output and $GITHUB_STEP_SUMMARY). Adding a write anywhere else needs a
// conscious decision, and an update of CLAUDE.md principle 2 and this list.
func TestOnlyDocumentedFilesWriteToDisk(t *testing.T) {
	allowed := map[string]bool{"cmd/envguard/init.go": true, "cmd/envguard/output.go": true}
	for _, dir := range []string{"cmd", "internal", "docs"} {
		err := filepath.WalkDir(filepath.Join(repoRoot, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			rel, _ := filepath.Rel(repoRoot, p)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/testutil/") {
				return nil
			}
			data, err := os.ReadFile(p) //nolint:gosec // test
			if err != nil {
				return err
			}
			if writeAPIs.Match(data) && !allowed[rel] {
				t.Errorf("%s writes to the filesystem; EnvGuard is read-only (CLAUDE.md principle 2)", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
