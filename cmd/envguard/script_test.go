package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain lets the test binary act as the envguard command inside
// testscript scripts, and isolates in-process tests from CI variables that
// change the default output (GitHub Actions auto-selects the github format).
func TestMain(m *testing.M) {
	// When the binary runs as the envguard command of a script, the
	// script's environment is deliberate: keep it.
	if name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"); name != "envguard" {
		for _, name := range []string{"GITHUB_ACTIONS", "GITHUB_STEP_SUMMARY", "NO_COLOR"} {
			_ = os.Unsetenv(name)
		}
	}
	testscript.Main(m, map[string]func(){
		"envguard": func() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) },
	})
}

// TestScripts runs the end-to-end CLI scripts in testdata/script/*.txtar.
// Each script covers one command: flags, output, exit codes and errors.
func TestScripts(t *testing.T) {
	demo := filepath.Join("..", "..", "examples", "demo-app")
	testscript.Run(t, testscript.Params{
		Dir: filepath.Join("testdata", "script"),
		Setup: func(env *testscript.Env) error {
			// A copy of the demo app outside any git repository, so results
			// do not depend on the git state of this repository.
			if err := copyTree(demo, filepath.Join(env.WorkDir, "demo")); err != nil {
				return err
			}
			env.Setenv("GITHUB_ACTIONS", "")
			env.Setenv("GITHUB_STEP_SUMMARY", "")
			env.Setenv("NO_COLOR", "")
			return nil
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			// exitcode N cmd args...: run cmd and require exit code N.
			"exitcode": func(ts *testscript.TestScript, neg bool, args []string) {
				if neg || len(args) < 2 {
					ts.Fatalf("usage: exitcode N command [args...]")
				}
				want, err := strconv.Atoi(args[0])
				if err != nil {
					ts.Fatalf("exitcode: %v", err)
				}
				got := 0
				if err := ts.Exec(args[1], args[2:]...); err != nil {
					var ee *exec.ExitError
					if !errors.As(err, &ee) {
						ts.Fatalf("exitcode: %v", err)
					}
					got = ee.ExitCode()
				}
				if got != want {
					ts.Fatalf("exit code %d, want %d", got, want)
				}
			},
		},
	})
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
