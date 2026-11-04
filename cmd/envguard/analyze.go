package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/mask"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/report"
	"github.com/ardamoustafa1/envguard/internal/rules"
)

// processEnvRules are the rules that make sense without env files.
var processEnvRules = []string{"EG001", "EG005", "EG014", "EG015"}

// analysisOptions select what is analyzed.
type analysisOptions struct {
	envs       []string
	app        string
	processEnv bool
	// skipRules builds the project from env files only, without rules, source
	// scanning, Compose, workflows or git (diff).
	skipRules bool
}

// analysis is the outcome of the shared check/doctor/diff pipeline.
type analysis struct {
	settings *config.Settings
	project  *project.Project
	app      string // resolved --app ("." when unset)
	findings []findings.Finding
	result   *rules.Result
	started  time.Time
}

// logger writes --verbose diagnostics to stderr. Messages contain paths,
// counts and fixed text only, never values.
type logger struct {
	w       io.Writer
	enabled bool
}

func (l logger) printf(format string, args ...any) {
	if l.enabled {
		// Paths in the messages come from the repository: keep terminals safe.
		_, _ = fmt.Fprintf(l.w, "envguard: %s\n", mask.Printable(fmt.Sprintf(format, args...)))
	}
}

func (l logger) phase(name string, start time.Time) {
	l.printf("timing: %-9s %s", name, time.Since(start).Round(10*time.Microsecond))
}

// analyze runs discovery, project building and (unless skipped) the rules.
func analyze(stderr io.Writer, settings *config.Settings, ao analysisOptions, started time.Time) (*analysis, error) {
	log := logger{w: stderr, enabled: settings.Verbose}
	if settings.ConfigPath != "" {
		log.printf("using config %s", settings.ConfigPath)
	} else {
		log.printf("no config file; using defaults")
	}
	log.phase("config", started)

	t := time.Now()
	inv, err := discovery.Discover(settings.DiscoveryOptions())
	if err != nil {
		return nil, err
	}
	for _, s := range inv.Skipped {
		log.printf("skipped %s (%s)", s.Path, s.Reason)
	}
	if inv.Truncated {
		_, _ = fmt.Fprintf(stderr, "envguard: warning: stopped after %d files; results are incomplete (narrow --root or add excludes)\n",
			discovery.DefaultMaxFiles)
	}
	log.phase("discover", t)

	app, err := resolveApp(ao.app, inv)
	if err != nil {
		return nil, err
	}
	opts := project.Options{Environments: ao.envs, EnvFilesOnly: ao.skipRules}
	if ao.processEnv {
		env := ""
		if len(ao.envs) == 1 {
			env = ao.envs[0]
		}
		opts.ProcessEnv = &project.ProcessEnv{Vars: environ(), Env: env, App: app}
		log.printf("--process-env: checking %d process variables against app %s", len(opts.ProcessEnv.Vars), app)
	}

	t = time.Now()
	p, err := project.Build(settings, inv, opts)
	if err != nil {
		return nil, err
	}
	log.phase("build", t)
	if err := checkEnvironments(p, ao.envs); err != nil {
		return nil, err
	}
	log.printf("analyzing %s", p)
	for _, f := range p.UnscannableFiles {
		log.printf("skipped source file %s (could not be read or scanned)", f)
	}
	if n := p.DynamicUsages("."); n > 0 {
		log.printf("%s could not be analyzed (variable names computed at run time)", plural(n, "dynamic lookup", "dynamic lookups"))
	}
	if !p.Git.Available {
		log.printf("git checks (EG008, EG009, tracked-file part of EG010) skipped: %s", p.Git.Reason)
	}
	a := &analysis{settings: settings, project: p, app: app, started: started}
	if ao.skipRules {
		return a, nil
	}

	if loc, bulk := rules.BulkEnvironmentRead(p); bulk {
		log.printf("EG004 skipped: the code reads the whole environment at %s, so which declared variables are unused cannot be known", loc)
	}
	for _, root := range rules.SkippedForUnused(p) {
		log.printf("EG004 skipped for %s: the scanner found no env var usages there (language not supported?)", root)
	}
	toRun := rules.All()
	if ao.processEnv {
		toRun = slices.DeleteFunc(toRun, func(r rules.Rule) bool { return !slices.Contains(processEnvRules, r.ID()) })
		log.printf("file-based rules are skipped in --process-env mode")
	}
	t = time.Now()
	res, err := rules.Run(p, toRun)
	if err != nil {
		return nil, err
	}
	log.phase("rules", t)
	log.printf("ran rules %v", res.Rules)
	if ao.app != "" && !ao.processEnv {
		res.Findings = slices.DeleteFunc(res.Findings, func(f findings.Finding) bool { return !underApp(f.File, app) })
	}
	a.result = res
	a.findings = res.Findings
	return a, nil
}

// checkEnvironments rejects --env values that name no environment of the
// repository. A typo such as --env prodction must not end in a green "no
// problems found" for an environment that was never checked. The error says
// which value is wrong by position, never by echoing it.
func checkEnvironments(p *project.Project, requested []string) error {
	if len(requested) == 0 {
		return nil
	}
	known := report.Environments(p)
	for i, e := range requested {
		if slices.Contains(known, discovery.NormalizeEnvironment(e)) {
			continue
		}
		have := "this repository has no env files or templates"
		if len(known) > 0 {
			have = "known environments: " + mask.Printable(strings.Join(known, ", ")) + " (aliases such as prod and dev are fine)"
		}
		return fmt.Errorf("--env: value %d names no environment of this repository; %s", i+1, have)
	}
	return nil
}

// failed applies a --fail-on threshold.
func failed(failOn string, fs []findings.Finding) error {
	if failOn == config.FailOnNever {
		return nil
	}
	threshold, err := findings.ParseSeverity(failOn)
	if err != nil {
		return fmt.Errorf("fail-on: %w", err)
	}
	if findings.AnyAtLeast(fs, threshold) {
		return errFindings
	}
	return nil
}

// resolveApp normalizes --app and checks that it is a discovered app root.
// The error lists the known app roots but never echoes the argument.
func resolveApp(arg string, inv *discovery.Inventory) (string, error) {
	if arg == "" {
		return ".", nil
	}
	app := path.Clean(filepath.ToSlash(arg))
	app = strings.TrimPrefix(app, "./")
	if app == "" {
		app = "."
	}
	if app == "." || slices.Contains(inv.AppRoots, app) {
		return app, nil
	}
	return "", fmt.Errorf("--app: not an app root under --root (directories with package.json, go.mod, ...); known app roots: %s",
		mask.Printable(strings.Join(inv.AppRoots, ", ")))
}

func underApp(file, app string) bool {
	return app == "." || file == project.ProcessEnvPath || strings.HasPrefix(file, app+"/")
}

// environ returns the process environment as a map. Values stay in memory
// and only ever reach output through internal/mask.
func environ() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			out[k] = v
		}
	}
	return out
}

var errUnknownFormat = errors.New("--format: unknown format")

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
