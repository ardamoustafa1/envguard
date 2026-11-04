package scanner

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

// TestFixtureProjects scans testdata/scanner, a tree with one project per
// language. Conventions: every name containing FAKE sits in a comment, a
// string, HTML or an assignment and must never be reported; every REAL_*
// token (except REAL_DYNAMIC, which only appears as a computed key) must be
// reported. The full result is snapshotted in testdata/scanner.golden.json.
// Regenerate with: go test ./internal/scanner -run TestFixtureProjects -update
func TestFixtureProjects(t *testing.T) {
	root := filepath.Join("testdata", "scanner")
	inv, err := discovery.Discover(discovery.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res := Scan(Options{Root: inv.Root, Files: inv.Files})
	if len(res.Unreadable) != 0 {
		t.Fatalf("unreadable: %v", res.Unreadable)
	}

	found := map[string]bool{}
	dynamic := 0
	for _, u := range res.Usages {
		if strings.Contains(u.Variable, "FAKE") {
			t.Errorf("phantom usage %s at %s:%d:%d (%s)", u.Variable, u.File, u.Line, u.Column, u.Pattern)
		}
		if u.Dynamic {
			dynamic++
		}
		found[u.Variable] = true
	}

	realRE := regexp.MustCompile(`\bREAL_[A-Z0-9_]+`)
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) == ".json" || filepath.Base(p) == "go.mod" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, name := range realRE.FindAllString(string(data), -1) {
			if name != "REAL_DYNAMIC" && !found[name] {
				t.Errorf("missed usage %s in %s", name, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if dynamic < 6 {
		t.Errorf("dynamic usages = %d, want at least 6 (one per fixture with computed keys)", dynamic)
	}

	got, err := json.MarshalIndent(res.Usages, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, filepath.Join("testdata", "scanner.golden.json"), append(got, '\n'))
}

func TestFrameworkNotes(t *testing.T) {
	root := filepath.Join("testdata", "scanner")
	inv, err := discovery.Discover(discovery.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	res := Scan(Options{Root: inv.Root, Files: inv.Files})
	notes := map[string]bool{}
	for _, u := range res.Usages {
		if u.Note != "" {
			notes[u.Variable] = true
		}
	}
	want := []string{"REAL_CLIENT_API_BASE", "REAL_NOT_EXPOSED", "REAL_SERVER_SECRET"}
	var got []string
	for n := range notes {
		got = append(got, n)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("notes on %v, want %v", got, want)
	}
}

func TestScanDeterministicAndWorkers(t *testing.T) {
	root := filepath.Join("testdata", "scanner")
	inv, err := discovery.Discover(discovery.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	one := Scan(Options{Root: inv.Root, Files: inv.Files, Workers: 1})
	many := Scan(Options{Root: inv.Root, Files: inv.Files, Workers: 16})
	a, _ := json.Marshal(one)
	b, _ := json.Marshal(many)
	if !bytes.Equal(a, b) {
		t.Error("results depend on worker count")
	}
	if one.Scanned == 0 {
		t.Error("nothing scanned")
	}
}

func TestScanUnreadable(t *testing.T) {
	files := []discovery.File{{Path: "missing.js", Kind: discovery.KindSource, Language: discovery.LangJavaScript, AppRoot: "."}}
	res := Scan(Options{Root: t.TempDir(), Files: files})
	if len(res.Unreadable) != 1 || res.Scanned != 0 {
		t.Errorf("res = %+v", res)
	}
}
