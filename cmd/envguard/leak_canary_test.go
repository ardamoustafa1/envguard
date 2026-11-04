package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/report"
	"github.com/ardamoustafa1/envguard/internal/rules"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

// canaryRepo builds a repository in which every value is a unique,
// secret-shaped canary containing "CNRY" (see testutil.Canary). Values are
// placed in every position that could plausibly leak: env values,
// duplicates with different values, invalid lines whose "key" is a canary,
// invalid escapes, unterminated quotes, inline comments, templates, a
// git-tracked env file, Compose and workflow values, source code defaults
// and comments.
func canaryRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	n := 0
	next := func() string { n++; return testutil.Canary(n) }
	files := map[string]string{}
	add := func(rel string, lines ...string) { files[rel] = strings.Join(lines, "\n") + "\n" }

	add("package.json", `{"name": "canary", "dependencies": {"next": "15"}}`)
	add(".env.example",
		"API_TOKEN="+next(),
		"DATABASE_URL="+next(),
		"OPTIONAL_THING= # optional",
		"SHARED_SECRET="+next(),
		"ONLY_IN_TEMPLATE=",
	)
	add(".env.production.example", "PROD_SECRET="+next(), "PROD_ONLY_TEMPLATE=")
	add(".env.staging.example", "STAGING_ONLY_TEMPLATE="+next())
	add(".env",
		"API_TOKEN="+next(),
		"API_TOKEN="+next(), // duplicate with a different value (EG006 evidence)
		"DATABASE_URL=",     // empty required (EG005)
		"UNDOCUMENTED="+next(),
		next(),          // a line whose "key" is a canary, without '='
		"1"+next()+"=x", // invalid key built from a canary
		`ESCAPED="`+next()+`\q"`,
		"COMMENTED="+next()+" # "+next(),
		"SPACED="+next()+" "+next(),
		"REF=${MISSING_"+"REF}/"+next(),
		`UNTERMINATED="`+next(),
	)
	add(".env.production",
		"API_TOKEN="+next(),
		"PROD_SECRET="+next(),
		"LEAKED_FILE_VALUE="+next(),
		"WORKERS="+next(),                                // EG014: not an int
		"NODE_ENV="+next(),                               // EG015: not "production"
		"CACHE_URL=postgres://u:"+next()+"@localhost/db", // EG015: localhost
		"PUBLIC_LABEL="+next(),                           // EG014 on a sensitive variable
	)
	add(".envguard.yaml",
		"version: 1",
		"profiles: [production-safety]",
		"variables:",
		"  API_TOKEN: { type: url, required: true }",
		"  WORKERS: { type: int, min: 1 }",
		"  PUBLIC_LABEL: { type: int, sensitive: true }",
		"  CACHE_URL: { type: url }",
		"  NODE_ENV: { type: string }",
	)
	add("docker-compose.yml",
		"services:",
		"  api:",
		"    environment:",
		`      SECRET: "`+next()+`"`,
		`      URL: "${UNDEFINED_IN_COMPOSE}/`+next()+`"`,
		"    env_file:",
		"      - ./missing.env",
	)
	add(".github/workflows/ci.yml",
		"on: push",
		"env:",
		`  WORKFLOW_VALUE: "`+next()+`"`,
		"jobs:",
		"  build:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		`      - run: echo "`+next()+`" ${{ secrets.UNDOCUMENTED_CI_SECRET }}`,
	)
	add("src/app.js",
		"// "+next(),
		`const a = process.env.API_TOKEN || "`+next()+`";`,
		`const b = process.env.NOT_DECLARED_ANYWHERE;`,
		`const c = "`+next()+`";`,
		"module.exports = { a, b, c };",
	)
	add("pages/index.js", `export default () => process.env.SERVER_ONLY_SECRET || "`+next()+`";`)

	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := exec.LookPath("git"); err == nil {
		testutil.InitGit(t, dir, []string{".env.production", ".env.example"})
	}
	return dir
}

func assertNoCanary(t *testing.T, what, out string) {
	t.Helper()
	if i := strings.Index(out, "CNRY"); i >= 0 {
		start, end := max(0, i-80), min(len(out), i+40)
		t.Errorf("%s leaks a canary value:\n...%s...", what, out[start:end])
	}
}

// TestLeakCanary is a permanent guard for principle 3 ("never print secret
// values"). It must keep passing forever: every new output path (reporter,
// command, log line, error) must be added here.
func TestLeakCanary(t *testing.T) {
	dir := canaryRepo(t)
	// The process environment holds canaries too (--process-env mode).
	for i, name := range []string{"API_TOKEN", "WORKERS", "NODE_ENV", "PUBLIC_LABEL", "CACHE_URL", "PROD_SECRET"} {
		t.Setenv(name, testutil.Canary(9500+i))
	}

	outDir := t.TempDir()
	blocker := filepath.Join(outDir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runs := [][]string{
		{"--root", dir, "--verbose", "check", "--fail-on", "info"},
		{"--root", dir, "--verbose", "--no-color", "check", "--env", "production,staging"},
		{"--root", dir, "check", "--quiet"},
		{"--root", dir, "--verbose", "doctor"},
		{"--root", dir, "--verbose"}, // no command = doctor
		{"--root", dir, "--verbose", "diff"},
		{"--root", dir, "--verbose", "diff", "production", "staging"},
		{"--root", dir, "diff", "default", "prod", "--format", "json"},
		{"--root", dir, "diff", "--format", "json"},
		{"--root", dir, "--verbose", "init", "--dry-run", "--example"},
		{"--root", dir, "--verbose", "check", "--process-env", "--env", "production"},
		{"--root", dir, "check", "--process-env", "--env", "production", "--format", "doctor"},
		{"--root", dir, "check", "--process-env", "--env", "production", "--format", "json"},
		{"--root", dir, "check", "--process-env", "--app", testutil.Canary(9003)},
		{"--root", dir, "check", "--fail-on", testutil.Canary(9001)},
		{"--root", dir, "check", "--env", testutil.Canary(9002)},
		{"--root", dir, "doctor", "--env", testutil.Canary(9004)},
		{"--root", dir, "check", "--format", testutil.Canary(9005)},
		{"--root", dir, "diff", "production", testutil.Canary(9006)},
		{"--root", dir, "explain", testutil.Canary(9007)},
		{"--root", dir, "check", "--output", filepath.Join(blocker, testutil.Canary(9008), "x.json")},
		{"--root", dir, "--ascii", "check"},
		{"--root", dir, "explain", "--list"},
	}
	for _, f := range report.Formats() {
		runs = append(runs,
			[]string{"--root", dir, "--verbose", "check", "--format", f},
			[]string{"--root", dir, "doctor", "--format", f},
			[]string{"--root", dir, "--ascii", "check", "--format", f, "--output", filepath.Join(outDir, "check."+f)},
			[]string{"--root", dir, "doctor", "--format", f, "--output", filepath.Join(outDir, "doctor."+f)},
		)
	}
	for _, r := range rules.All() {
		runs = append(runs, []string{"--root", dir, "explain", r.ID()})
	}
	var combined strings.Builder
	runAll := func(runs [][]string) {
		for _, args := range runs {
			var stdout, stderr bytes.Buffer
			run(args, &stdout, &stderr)
			name := strings.Join(args[2:], " ")
			assertNoCanary(t, "stdout of "+name, stdout.String())
			assertNoCanary(t, "stderr of "+name, stderr.String())
			combined.WriteString(stdout.String())
		}
	}
	runAll(runs)

	// GitHub Actions: auto-selected annotations plus the job summary file.
	summary := filepath.Join(outDir, "summary.md")
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	runAll([][]string{
		{"--root", dir, "--verbose", "check"},
		{"--root", dir, "doctor", "--format", "github"},
		{"--root", dir, "check", "--process-env", "--env", "production"},
	})
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITHUB_STEP_SUMMARY", "")

	written, err := filepath.Glob(filepath.Join(outDir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(written) < 2*len(report.Formats())+1 {
		t.Errorf("expected report files and a job summary, got %v", written)
	}
	for _, f := range written {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		assertNoCanary(t, filepath.Base(f), string(data))
	}
	for _, want := range []string{"\"schema_version\": 1", "\"version\": \"2.1.0\"", "::error file=", "EnvGuard doctor"} {
		if !strings.Contains(combined.String(), want) {
			t.Errorf("combined output lacks %q; a format is not exercised", want)
		}
	}

	// A config file whose values are canaries: errors must not echo them.
	cfgDir := canaryRepo(t)
	cfg := fmt.Sprintf("version: 1\nfail_on: %s\nrules:\n  EG001: %s\nvariables:\n  X:\n    pattern: \"(%s\"\n",
		testutil.Canary(9100), testutil.Canary(9101), testutil.Canary(9102))
	if err := os.WriteFile(filepath.Join(cfgDir, ".envguard.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", cfgDir, "check"}, &stdout, &stderr); code != exitError {
		t.Errorf("invalid config: exit %d", code)
	}
	assertNoCanary(t, "config error", stdout.String()+stderr.String())

	// Every field of every finding, marshaled raw (Evidence and Related
	// included), in case a reporter adds a field later.
	settings, err := config.Load(config.Flags{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := discovery.Discover(settings.DiscoveryOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, err := project.Build(settings, inv, project.Options{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rules.Run(p, rules.All())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	assertNoCanary(t, "findings JSON", string(raw))

	// The fixture must actually exercise the risky paths, or the test proves
	// nothing.
	fired := map[string]bool{}
	evidence := false
	for _, f := range res.Findings {
		fired[f.RuleID] = true
		evidence = evidence || f.Evidence != ""
	}
	// Every rule must be exercised, so a new rule cannot ship with an output
	// path the canary never looked at. The git-based ones need git.
	for _, r := range rules.All() {
		switch r.ID() {
		case "EG008", "EG009":
			if p.Git.Available && !fired[r.ID()] {
				t.Errorf("canary fixture does not trigger %s", r.ID())
			}
		default:
			if !fired[r.ID()] {
				t.Errorf("canary fixture does not trigger %s; extend it so that rule's output is covered", r.ID())
			}
		}
	}
	if !evidence {
		t.Error("no finding carries Evidence; the masked-evidence path is not covered")
	}
	if !strings.Contains(combined.String(), "EG006") {
		t.Error("plain output does not include the findings")
	}
}

// panickingRule fails in a way that could carry a value into the error text.
type panickingRule struct{ boom func() }

func (panickingRule) ID() string                         { return "EG999" }
func (panickingRule) Name() string                       { return "panics" }
func (panickingRule) DefaultSeverity() findings.Severity { return findings.Error }
func (r panickingRule) Check(*project.Project) []findings.Finding {
	r.boom()
	return nil
}

// TestLeakCanaryPanics: a bug in a rule must produce an internal error that
// never contains the value the rule was looking at, whatever it panicked with.
func TestLeakCanaryPanics(t *testing.T) {
	dir := canaryRepo(t)
	settings, err := config.Load(config.Flags{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := discovery.Discover(settings.DiscoveryOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, err := project.Build(settings, inv, project.Options{})
	if err != nil {
		t.Fatal(err)
	}
	canary := testutil.Canary(7777)
	var nilMap map[string]int
	for name, boom := range map[string]func(){
		"string":      func() { panic(canary) },
		"error":       func() { panic(errors.New(canary)) },
		"formatted":   func() { panic(fmt.Sprintf("bad value %q", canary)) },
		"nil map key": func() { nilMap[canary] = 1 },
		"index":       func() { _ = []string{}[len(canary)] },
	} {
		_, err := rules.Run(p, []rules.Rule{panickingRule{boom}})
		var ie *rules.InternalError
		if !errors.As(err, &ie) {
			t.Errorf("%s: want an *InternalError, got %v", name, err)
			continue
		}
		assertNoCanary(t, "internal error text ("+name+")", err.Error())
		assertNoCanary(t, "internal error stack ("+name+")", ie.Reason)
		var stderr bytes.Buffer
		code := runGuarded(&stderr, func() int { panic(canary) })
		if code != exitError {
			t.Errorf("runGuarded exit code %d", code)
		}
		assertNoCanary(t, "top-level panic guard", stderr.String())
	}
}
