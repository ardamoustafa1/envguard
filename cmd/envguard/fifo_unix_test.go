//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A subprocess bounds regressions: opening a FIFO without a writer blocks.
func TestAuxiliaryFIFOsNeverBlock(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path string
		code int
	}{
		{".envguard.yaml", exitError},
		{".envguard.yml", exitError},
		{".git/info/exclude", exitOK},
		{"package.json", exitOK},
	} {
		t.Run(tt.path, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("const x = process.env.REAL;\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fifo := filepath.Join(root, filepath.FromSlash(tt.path))
			if err := os.MkdirAll(filepath.Dir(fifo), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(fifo, 0o600); err != nil {
				t.Skipf("mkfifo: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestFIFOCommandHelper$")
			cmd.Env = append(os.Environ(), "ENVGUARD_FIFO_TEST_ROOT="+root)
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("command blocked on %s: %v", tt.path, ctx.Err())
			}
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if code != tt.code {
				t.Errorf("exit = %d, want %d: %s", code, tt.code, out)
			}
		})
	}
}

func TestFIFOCommandHelper(t *testing.T) {
	root := os.Getenv("ENVGUARD_FIFO_TEST_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	os.Exit(run([]string{"--root", root, "check", "--fail-on", "never", "--format", "json"}, os.Stdout, os.Stderr))
}
