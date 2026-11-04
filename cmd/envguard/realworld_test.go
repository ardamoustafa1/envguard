package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/testutil"
)

var realworld = filepath.Join(repoRoot, "testdata", "realworld")

// realworldRepo copies a fixture to a temp dir and stages it in a fresh git
// repository the way a clone would look: ignored files (a real `.env`) stay
// untracked, everything else is added.
func realworldRepo(t *testing.T, name string) string {
	t.Helper()
	dir := testutil.CopyFixture(t, filepath.Join(realworld, name))
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, args := range [][]string{{"-c", "init.defaultBranch=main", "init", "-q"}, {"add", "-A"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // test
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func realworldNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(realworld)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		t.Fatal("no real-world fixtures")
	}
	return names
}

// TestRealWorldGolden checks every fixture project (testdata/realworld/*,
// written like real open-source projects) against a reviewed golden. A
// finding in the golden has been checked by hand and is a true positive;
// anything else a change produces must be reviewed before it is accepted.
// Update with: go test ./cmd/envguard -run TestRealWorldGolden -update
func TestRealWorldGolden(t *testing.T) {
	for _, name := range realworldNames(t) {
		t.Run(name, func(t *testing.T) {
			dir := realworldRepo(t, name)
			var stdout, stderr bytes.Buffer
			code := run([]string{"--root", dir, "check", "--fail-on", "never", "--no-color"}, &stdout, &stderr)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("unexpected stderr: %s", stderr.String())
			}
			testutil.Golden(t, filepath.Join("testdata", "realworld", name+".golden"), stdout.Bytes())
		})
	}
}

// TestRealWorldPlantedProblems checks recall: every problem a fixture's
// PLANTED.txt lists ("RULE path VARIABLE-or-dash - why") must be reported.
func TestRealWorldPlantedProblems(t *testing.T) {
	for _, name := range realworldNames(t) {
		data, err := os.ReadFile(filepath.Join(realworld, name, "PLANTED.txt"))
		if err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			dir := realworldRepo(t, name)
			var stdout, stderr bytes.Buffer
			run([]string{"--root", dir, "check", "--fail-on", "never", "--format", "json"}, &stdout, &stderr)
			var doc struct {
				Findings []struct {
					RuleID   string `json:"rule_id"`
					Variable string `json:"variable"`
					Location struct {
						File string `json:"file"`
					} `json:"location"`
				} `json:"findings"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(data), "\n") {
				f := strings.Fields(line)
				if len(f) < 3 || !strings.HasPrefix(f[0], "EG") {
					continue
				}
				rule, path, variable := f[0], f[1], f[2]
				found := false
				for _, got := range doc.Findings {
					if got.RuleID == rule && got.Location.File == path && (variable == "-" || got.Variable == variable) {
						found = true
					}
				}
				if !found {
					t.Errorf("planted problem not reported: %s", line)
				}
			}
		})
	}
}

// TestRealWorldEmptyAndEnvless covers the degenerate repositories: an empty
// directory must work in every command and format, with a friendly result.
func TestRealWorldEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"check"}, {"doctor"}, {"diff"}, {"check", "--format", "json"}, {"check", "--format", "sarif"},
		{"check", "--format", "github"}, {"doctor", "--format", "json"}, {"init", "--dry-run"},
		{"check", "--process-env"}, {"--verbose", "check"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(append([]string{"--root", dir}, args...), &stdout, &stderr); code != exitOK {
			t.Errorf("envguard %s: exit %d: %s", strings.Join(args, " "), code, stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), "panic") {
			t.Errorf("envguard %s printed a panic", strings.Join(args, " "))
		}
	}
	var stdout, stderr bytes.Buffer
	run([]string{"--root", dir, "check"}, &stdout, &stderr)
	if got := stdout.String(); got != "✓ No problems found · 0 files\n" {
		t.Errorf("check on an empty directory printed %q", got)
	}
	stdout.Reset()
	run([]string{"--root", dir, "doctor"}, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "All good") {
		t.Errorf("doctor on an empty directory:\n%s", stdout.String())
	}
}

// convertText rewrites every text file of the repository with Windows line
// endings (and optionally a UTF-8 BOM on env and YAML files), as a checkout
// with core.autocrlf=true would.
func convertText(t *testing.T, dir string, bom bool) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if err == nil && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // test
		if err != nil {
			return err
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		data = bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
		name := d.Name()
		if bom && (strings.HasPrefix(name, ".env") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")) {
			data = append([]byte("\xef\xbb\xbf"), data...)
		}
		return os.WriteFile(p, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWindowsLineEndingsAndBOM: the same repository with CRLF line endings,
// and with CRLF plus a BOM, gives exactly the same findings at the same
// lines and columns as with LF.
func TestWindowsLineEndingsAndBOM(t *testing.T) {
	type finding struct {
		RuleID   string `json:"rule_id"`
		Message  string `json:"message"`
		Evidence string `json:"evidence"`
		Location struct {
			File   string `json:"file"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"location"`
	}
	collect := func(dir string) []finding {
		var stdout, stderr bytes.Buffer
		run([]string{"--root", dir, "check", "--format", "json", "--fail-on", "never"}, &stdout, &stderr)
		var doc struct {
			Findings []finding `json:"findings"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
			t.Fatalf("%v\n%s", err, stderr.String())
		}
		return doc.Findings
	}
	lf := collect(demoRepo(t))
	if len(lf) < 20 {
		t.Fatalf("only %d findings in the LF repository", len(lf))
	}
	for _, bom := range []bool{false, true} {
		dir := demoRepo(t)
		convertText(t, dir, bom)
		got := collect(dir)
		if len(got) != len(lf) {
			t.Fatalf("bom=%v: %d findings, want %d", bom, len(got), len(lf))
		}
		for i := range lf {
			if got[i] != lf[i] {
				t.Errorf("bom=%v: finding %d differs:\n  LF:   %+v\n  CRLF: %+v", bom, i, lf[i], got[i])
			}
		}
	}
}
