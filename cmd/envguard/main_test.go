package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, exitOK},
		{"findings", errFindings, exitFindings},
		{"wrapped findings", fmt.Errorf("check: %w", errFindings), exitFindings},
		{"diff --exit-code", errDifferences, exitFindings},
		{"invalid config", fmt.Errorf("load: %w", config.ErrInvalid), exitError},
		{"other", errors.New("boom"), exitError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCode(tt.err); got != tt.want {
				t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	emptyDir := t.TempDir()
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"no args runs doctor", []string{"--root", emptyDir}, exitOK, "All good", ""},
		{"help", []string{"--help"}, exitOK, "Usage:", ""},
		{"version", []string{"version"}, exitOK, "envguard dev\ncommit: none\nbuilt:  unknown\n", ""},
		{"check on a tree without env files", []string{"--root", emptyDir, "check"}, exitOK, "✓ No problems found · 0 files", ""},
		{"doctor", []string{"--root", emptyDir, "doctor"}, exitOK, "EnvGuard doctor", ""},
		{"diff", []string{"--root", emptyDir, "diff"}, exitOK, "No env files or templates found.", ""},
		{"init dry-run on an empty tree", []string{"--root", emptyDir, "init", "--dry-run"}, exitOK, "No variables found yet", ""},
		{"explain", []string{"explain", "EG001"}, exitOK, "EG001 missing-variable", ""},
		{"unknown command", []string{"nope"}, exitError, "", "unknown command"},
		{"unknown flag", []string{"--nope"}, exitError, "", "unknown flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tt.args, &stdout, &stderr)
			if got != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", got, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunWithConfig(t *testing.T) {
	writeConfig := func(t *testing.T, content string) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".envguard.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	valid := writeConfig(t, "version: 1\n")
	invalid := writeConfig(t, "version: 1\nexlude: [x]\n")

	tests := []struct {
		name       string
		args       []string
		wantStderr []string
	}{
		{"valid config reaches command", []string{"--root", valid, "--no-color", "--verbose", "check"}, nil},
		{"invalid config", []string{"--root", invalid, "check"}, []string{"invalid configuration", ".envguard.yaml:2:1", `did you mean "exclude"?`}},
		{"invalid config doctor", []string{"--root", invalid, "doctor"}, []string{"invalid configuration"}},
		{"invalid config diff", []string{"--root", invalid, "diff"}, []string{"invalid configuration"}},
		{"explicit missing config", []string{"--root", valid, "--config", filepath.Join(valid, "nope.yaml"), "check"}, []string{"config file not found"}},
		{"missing root", []string{"--root", filepath.Join(valid, "missing"), "check"}, []string{"root"}},
		{"bad fail-on", []string{"--root", valid, "check", "--fail-on", "warn"}, []string{"--fail-on", `did you mean "warning"?`}},
		{"good fail-on", []string{"--root", valid, "check", "--fail-on", "warning"}, nil},
		{"version ignores invalid config", []string{"--root", invalid, "version"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			wantCode := exitError
			if tt.wantStderr == nil {
				wantCode = exitOK
			}
			if code != wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, wantCode, stderr.String())
			}
			for _, w := range tt.wantStderr {
				if !strings.Contains(stderr.String(), w) {
					t.Errorf("stderr %q does not contain %q", stderr.String(), w)
				}
			}
		})
	}
}
