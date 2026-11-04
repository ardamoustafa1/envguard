package gitutil

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Timeout bounds each git invocation.
const Timeout = 10 * time.Second

// Status is the git view of the root. When Available is false, Reason says
// why (git not installed, not a repository, ...) and the maps are empty:
// rules that need git must skip, never guess.
type Status struct {
	Available bool
	Reason    string
	// Tracked holds every file in the index, relative to the root and
	// slash-separated.
	Tracked map[string]bool
	// Ignored holds the queried paths that .gitignore rules ignore. Tracked
	// files are never reported as ignored.
	Ignored map[string]bool
}

// Inspect asks git which files under root are tracked and which of paths
// (relative, slash-separated) are ignored. It runs `git ls-files -z` and one
// batched `git check-ignore -z --stdin`. It never fails: problems make the
// status unavailable.
func Inspect(ctx context.Context, root string, paths []string) *Status {
	return inspect(ctx, "git", root, paths)
}

func inspect(ctx context.Context, gitName, root string, paths []string) *Status {
	st := &Status{Tracked: map[string]bool{}, Ignored: map[string]bool{}}
	bin, err := exec.LookPath(gitName)
	if err != nil {
		st.Reason = "git is not installed"
		return st
	}
	out, _, err := run(ctx, bin, root, nil, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		st.Reason = "not a git repository (or not accessible)"
		return st
	}

	out, _, err = run(ctx, bin, root, nil, "ls-files", "-z", "--cached")
	if err != nil {
		st.Reason = "git ls-files failed"
		return st
	}
	for _, p := range splitZ(out) {
		st.Tracked[filepath.ToSlash(p)] = true
	}

	if len(paths) > 0 {
		var in bytes.Buffer
		for _, p := range paths {
			in.WriteString(p)
			in.WriteByte(0)
		}
		out, code, err := run(ctx, bin, root, in.Bytes(), "check-ignore", "-z", "--stdin")
		// Exit code 1 means "nothing is ignored" and is not an error.
		if err != nil && code != 1 {
			st.Reason = "git check-ignore failed"
			st.Tracked = map[string]bool{}
			return st
		}
		for _, p := range splitZ(out) {
			st.Ignored[filepath.ToSlash(p)] = true
		}
	}
	st.Available = true
	return st
}

// run executes git with a fixed argument list (never through a shell).
// Configuration that could execute programs from the inspected repository
// (core.fsmonitor) is disabled, optional locks are off so nothing is
// written, and prompts are disabled.
func run(ctx context.Context, bin, root string, stdin []byte, args ...string) (out []byte, code int, err error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	full := append([]string{
		"-C", root,
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.quotePath=false",
	}, args...)
	cmd := exec.CommandContext(ctx, bin, full...) // #nosec G204 -- fixed argument list, no shell
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // never surface git's messages: they may quote paths or config
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return stdout.Bytes(), code, err
}

func splitZ(b []byte) []string {
	var out []string
	for _, part := range bytes.Split(b, []byte{0}) {
		if len(part) > 0 {
			out = append(out, string(part))
		}
	}
	return out
}
