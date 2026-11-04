package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/report"
)

// reportFlags are shared by check and doctor.
type reportFlags struct {
	format string
	output string
}

func (rf *reportFlags) add(cmd *cobra.Command, def string) {
	f := cmd.Flags()
	f.StringVar(&rf.format, "format", def, "output format: "+strings.Join(report.Formats(), ", "))
	f.StringVarP(&rf.output, "output", "o", "", "write the report to this file instead of stdout")
}

// resolveFormat picks the format: the flag if set, github inside GitHub
// Actions for check, else the command default. The error never echoes the
// argument.
func resolveFormat(cmd *cobra.Command, value, def string, allowed []string, autoGitHub bool) (string, error) {
	if !cmd.Flags().Changed("format") {
		if autoGitHub && os.Getenv("GITHUB_ACTIONS") == "true" {
			return report.FormatGitHub, nil
		}
		return def, nil
	}
	f := strings.ToLower(strings.TrimSpace(value))
	if !slices.Contains(allowed, f) {
		return "", fmt.Errorf("%w; want one of %s", errUnknownFormat, strings.Join(allowed, ", "))
	}
	return f, nil
}

// terminal describes where output goes.
type terminal struct {
	opts    report.Options
	restore func()
}

// detectTerminal decides color, Unicode and width for w. Colors need a TTY
// and neither --no-color nor NO_COLOR nor TERM=dumb. Unicode symbols are
// used unless --ascii, or the TTY looks unable to render them (TERM=dumb,
// a non-UTF-8 locale, or a legacy Windows console).
func detectTerminal(w io.Writer, g *globalOptions) terminal {
	t := terminal{restore: func() {}}
	t.opts.ASCII = g.ASCII
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return t
	}
	t.opts.ShowDuration = true
	if width, _, err := term.GetSize(f.Fd()); err == nil {
		t.opts.Width = width
	}
	dumb := os.Getenv("TERM") == "dumb"
	if !t.opts.ASCII && (dumb || !unicodeTerminal()) {
		t.opts.ASCII = true
	}
	if g.NoColor || os.Getenv("NO_COLOR") != "" || dumb {
		return t
	}
	restore, err := termenv.EnableVirtualTerminalProcessing(termenv.NewOutput(f))
	if err != nil {
		return t // a console without ANSI support: stay plain
	}
	t.opts.Color = true
	t.restore = func() { _ = restore() }
	return t
}

func unicodeTerminal() bool {
	if runtime.GOOS == "windows" {
		return os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != "" ||
			os.Getenv("ConEmuANSI") == "ON" || strings.Contains(os.Getenv("TERM"), "xterm")
	}
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(name); v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	return true
}

// emit renders rep to stdout or --output and, for the github format inside
// GitHub Actions, appends a Markdown job summary to $GITHUB_STEP_SUMMARY.
func emit(stdout io.Writer, g *globalOptions, rf *reportFlags, format string, rep *report.Report) error {
	write := func(w io.Writer, opts report.Options) error { return report.Write(w, format, rep, opts) }
	if err := writeOutput(stdout, g, rf.output, write); err != nil {
		return err
	}
	if format == report.FormatGitHub {
		if p := os.Getenv("GITHUB_STEP_SUMMARY"); p != "" {
			if err := appendTo(p, func(w io.Writer) error { return report.GitHubSummary(w, rep) }); err != nil {
				return fmt.Errorf("write GitHub job summary: %w", err)
			}
		}
	}
	return nil
}

// writeOutput calls write with stdout (terminal-aware options) or, with
// --output, a new file (parent directories are created; no colors, Unicode
// unless --ascii).
func writeOutput(stdout io.Writer, g *globalOptions, output string, write func(io.Writer, report.Options) error) error {
	if output == "" {
		t := detectTerminal(stdout, g)
		defer t.restore()
		return write(stdout, t.opts)
	}
	output = filepath.Clean(output)
	if dir := filepath.Dir(output); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("--output: %w", pathErr(err))
		}
	}
	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("--output: %w", pathErr(err))
	}
	werr := write(f, report.Options{ASCII: g.ASCII})
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		return fmt.Errorf("--output: %w", pathErr(err))
	}
	return nil
}

func appendTo(p string, write func(io.Writer) error) error {
	// p is $GITHUB_STEP_SUMMARY, a file the CI runner provides for this.
	f, err := os.OpenFile(filepath.Clean(p), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 G703 -- runner-provided path
	if err != nil {
		return pathErr(err)
	}
	werr := write(f)
	return errors.Join(werr, f.Close())
}

// pathErr strips the path from a *PathError (it may have come from an
// environment variable); the operation and reason remain.
func pathErr(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s: %w", pe.Op, pe.Err)
	}
	return err
}

// buildReport assembles the report of an analysis.
func buildReport(a *analysis, command, version string, g *globalOptions, envs []string, app string) *report.Report {
	// Only known environment names are shown: an unknown --env value is
	// user input and is never echoed.
	known := report.Environments(a.project)
	var canon []string
	for _, e := range envs {
		if n := discovery.NormalizeEnvironment(e); slices.Contains(known, n) && !slices.Contains(canon, n) {
			canon = append(canon, n)
		}
	}
	root := g.Root
	if root == "" {
		root = "."
	}
	return report.Build(a.project, a.findings, report.Meta{
		Command:      command,
		Version:      version,
		Root:         filepath.ToSlash(root),
		App:          appFilter(app, a.app),
		Environments: canon,
		Suppressed:   a.result.Suppressed,
		Duration:     time.Since(a.started),
	})
}

func appFilter(arg, resolved string) string {
	if arg == "" {
		return ""
	}
	return resolved
}
