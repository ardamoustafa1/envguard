// Command envguard catches environment-variable and configuration mistakes
// before an app runs or deploys.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
)

// Build information, injected via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Exit codes. See CLAUDE.md section 6.
const (
	exitOK       = 0
	exitFindings = 1
	exitError    = 2
)

// errFindings signals that findings at or above the --fail-on threshold were
// reported. The findings themselves have already been printed.
var errFindings = errors.New("findings at or above threshold")

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	return runGuarded(stderr, func() int {
		cmd := newRootCmd(currentBuildInfo())
		cmd.SetArgs(args)
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)

		err := cmd.Execute()
		if err != nil && !errors.Is(err, errFindings) {
			_, _ = fmt.Fprintf(stderr, "envguard: %v\n", err)
		}
		return exitCode(err)
	})
}

// runGuarded turns a panic anywhere in the command into a one-line error and
// exit code 2, instead of Go's crash dump. The panic value is printed only
// for runtime errors (index out of range, nil dereference, ...), which never
// contain user data; any other value could, so only its type is shown.
func runGuarded(stderr io.Writer, fn func() int) (code int) {
	defer func() {
		if v := recover(); v != nil {
			_, _ = fmt.Fprintf(stderr, "envguard: internal error: %s; this is a bug, please report it\n", panicText(v))
			code = exitError
		}
	}()
	return fn()
}

func panicText(v any) string {
	if re, ok := v.(runtime.Error); ok {
		return re.Error()
	}
	return fmt.Sprintf("unexpected panic of type %T", v)
}

// currentBuildInfo combines -ldflags values with the Go build info.
func currentBuildInfo() buildInfo {
	return resolveBuildInfo(buildInfo{Version: version, Commit: commit, Date: date}, debug.ReadBuildInfo)
}

// exitCode is the single place that maps errors to exit codes.
func exitCode(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, errFindings):
		return exitFindings
	default:
		return exitError
	}
}
