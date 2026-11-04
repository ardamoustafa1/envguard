package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/testutil"
)

var fixtures = filepath.Join("..", "..", "testdata", "fixtures")

// TestCheckGolden snapshots the plain `check` output over every rule
// fixture. Regenerate with: go test ./cmd/envguard -run TestCheckGolden -update
func TestCheckGolden(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// A copy, so the git state of this repository cannot change the result.
	root := testutil.CopyFixture(t, fixtures)
	code := run([]string{"--root", root, "check"}, &stdout, &stderr)
	if code != exitFindings {
		t.Errorf("exit code = %d, want %d (stderr: %s)", code, exitFindings, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr: %s", stderr.String())
	}
	testutil.Golden(t, filepath.Join("testdata", "check.golden"), stdout.Bytes())
}

func TestCheckFlags(t *testing.T) {
	clean := t.TempDir()
	for name, content := range map[string]string{".env.example": "A=\n", ".env": "A=1\n"} {
		if err := os.WriteFile(filepath.Join(clean, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	warnOnly := filepath.Join(fixtures, "EG013")

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string
		emptyOut   bool
	}{
		{"clean repo", []string{"--root", clean, "check"}, exitOK, []string{"✓ No problems found · 2 files"}, false},
		{"warnings pass default threshold", []string{"--root", warnOnly, "check"}, exitOK, []string{"⚠ warning  EG013", "✗ 0 errors · ⚠ 2 warnings · 0 info"}, false},
		{"fail-on warning", []string{"--root", warnOnly, "check", "--fail-on", "warning"}, exitFindings, nil, false},
		{"fail-on info", []string{"--root", warnOnly, "check", "--fail-on=info"}, exitFindings, nil, false},
		{"fail-on never", []string{"--root", fixtures, "check", "--fail-on", "never"}, exitOK, []string{"✗ error"}, false},
		{"quiet prints nothing", []string{"--root", fixtures, "check", "--quiet"}, exitFindings, nil, true},
		{"quiet clean", []string{"--root", clean, "check", "--quiet"}, exitOK, nil, true},
		{"env filter with alias", []string{"--root", filepath.Join(fixtures, "EG001"), "check", "--env", "prod"},
			exitFindings, []string{"positive/.env.production\n", "✗ error    EG001  STRIPE_SECRET_KEY is declared"}, false},
		{"suppressed count", []string{"--root", filepath.Join(fixtures, "EG001"), "check"}, exitFindings, []string{"· 2 suppressed"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tt.wantCode, stdout.String(), stderr.String())
			}
			for _, w := range tt.wantStdout {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout does not contain %q:\n%s", w, stdout.String())
				}
			}
			if tt.emptyOut && stdout.Len() != 0 {
				t.Errorf("stdout should be empty, got %q", stdout.String())
			}
			if tt.name == "env filter with alias" && strings.Contains(stdout.String(), "positive/.env\n") {
				t.Error("--env prod still reports the default environment")
			}
		})
	}
}

func TestCheckVerbose(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"--root", fixtures, "--verbose", "check"}, &stdout, &stderr)
	for _, w := range []string{"no config file; using defaults", "analyzing", "ran rules [EG001"} {
		if !strings.Contains(stderr.String(), w) {
			t.Errorf("verbose stderr missing %q:\n%s", w, stderr.String())
		}
	}
}

func TestCheckConfigOverrides(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".env.example":   "A=\n",
		".env":           "A=1\nA=2\nB=1\n",
		".envguard.yaml": "version: 1\nfail_on: warning\nrules:\n  EG006: off\nignore:\n  - variable: B\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", dir, "check"}, &stdout, &stderr)
	if code != exitOK || !strings.Contains(stdout.String(), "✓ No problems found · 2 files · 1 suppressed") {
		t.Errorf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	// --fail-on beats fail_on from the config.
	stdout.Reset()
	if code := run([]string{"--root", dir, "check", "--fail-on", "error"}, &stdout, &stderr); code != exitOK {
		t.Errorf("code = %d", code)
	}
}
