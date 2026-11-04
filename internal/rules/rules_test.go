package rules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

var fixturesDir = filepath.Join("..", "..", "testdata", "fixtures")

// loadProject copies a fixture tree to a temporary directory (see
// testutil.CopyFixture: _gitignore renaming, {{secret:KIND}} expansion and
// git-tracked.txt setup) and builds a project from the copy, so results never
// depend on the git state of this repository.
func loadProject(t *testing.T, fixture string, mutate func(*config.Settings), envs ...string) *project.Project {
	t.Helper()
	root := testutil.CopyFixture(t, fixture)
	settings, err := config.Load(config.Flags{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(settings)
	}
	inv, err := discovery.Discover(settings.DiscoveryOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, err := project.Build(settings, inv, project.Options{Environments: envs})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// dump renders findings with every field, so goldens capture the wording of
// Message, Why and Fix as well as locations.
func dump(fs []findings.Finding, suppressed int) string {
	var b strings.Builder
	for i := range fs {
		f := &fs[i]
		fmt.Fprintf(&b, "%s  %s  %s %s  %s\n", f.Location(), f.Severity, f.RuleID, f.RuleName, f.Message)
		if f.Variable != "" {
			fmt.Fprintf(&b, "  variable: %s\n", f.Variable)
		}
		if f.Evidence != "" {
			fmt.Fprintf(&b, "  evidence: %s\n", f.Evidence)
		}
		fmt.Fprintf(&b, "  why: %s\n", f.Why)
		fmt.Fprintf(&b, "  fix: %s\n", strings.ReplaceAll(strings.TrimRight(f.Fix, "\n"), "\n", "\n       "))
		for _, r := range f.Related {
			fmt.Fprintf(&b, "  related: %s\n", r)
		}
		fmt.Fprintf(&b, "  docs: %s\n", f.DocsURL)
	}
	fmt.Fprintf(&b, "(%d findings, %d suppressed)\n", len(fs), suppressed)
	return b.String()
}

// TestRuleFixtures runs every registered rule on its own fixture
// testdata/fixtures/<ID>/ and compares the output with <ID>/golden.txt.
// Each fixture has a positive/ part (the rule must produce findings) and a
// negative/ part (no finding may point into it). Findings may also point
// outside both (e.g. root-level .github/workflows). Fixtures with a
// git-tracked.txt run in a fresh git repository. Regenerate with:
//
//	go test ./internal/rules -run TestRuleFixtures -update
func TestRuleFixtures(t *testing.T) {
	for _, r := range All() {
		t.Run(r.ID(), func(t *testing.T) {
			dir := filepath.Join(fixturesDir, r.ID())
			for _, sub := range []string{"positive", "negative"} {
				if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
					t.Fatalf("rule %s has no fixture %s/%s/", r.ID(), dir, sub)
				}
			}
			p := loadProject(t, dir, nil)
			res, err := Run(p, []Rule{r})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Findings) == 0 {
				t.Fatalf("rule %s produced no findings on its positive fixture", r.ID())
			}
			for _, f := range res.Findings {
				if strings.HasPrefix(f.File, "negative/") {
					t.Errorf("finding in negative/ (negative case failed): %s %s", f.Location(), f.Message)
				}
				if f.Why == "" || f.Fix == "" || f.Message == "" {
					t.Errorf("finding without Message/Why/Fix: %+v", f)
				}
				if f.RuleID != r.ID() || f.DocsURL == "" || f.Severity == 0 {
					t.Errorf("engine did not fill metadata: %+v", f)
				}
			}
			testutil.Golden(t, filepath.Join(dir, "golden.txt"), []byte(dump(res.Findings, res.Suppressed)))
		})
	}
}

func TestRegistry(t *testing.T) {
	all := All()
	if len(all) == 0 {
		t.Fatal("no rules registered")
	}
	for i, r := range all {
		if i > 0 && all[i-1].ID() >= r.ID() {
			t.Errorf("All() not sorted at %s", r.ID())
		}
		if r.Name() == "" || r.DefaultSeverity() == 0 {
			t.Errorf("%s: missing name or severity", r.ID())
		}
		doc := filepath.Join("..", "..", "docs", "rules", r.ID()+"-"+r.Name()+".md")
		if _, err := os.Stat(doc); err != nil {
			t.Errorf("%s: missing documentation %s", r.ID(), doc)
		}
		if got, ok := Lookup(strings.ToLower(r.ID())); !ok || got.ID() != r.ID() {
			t.Errorf("Lookup(%s) failed", r.ID())
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("duplicate Register did not panic")
		}
	}()
	Register(all[0])
}

type fakeRule struct {
	base
	check func(*project.Project) []findings.Finding
}

func (f fakeRule) Check(p *project.Project) []findings.Finding { return f.check(p) }

func TestRunRecoversPanics(t *testing.T) {
	p := loadProject(t, filepath.Join(fixturesDir, "EG006"), nil)
	boom := fakeRule{base{"EG900", "boom", findings.Error}, func(p *project.Project) []findings.Finding {
		_ = p.EnvFiles[len(p.EnvFiles)+1] // index out of range: runtime error
		return nil
	}}
	secretPanic := fakeRule{base{"EG901", "secret-panic", findings.Error}, func(*project.Project) []findings.Finding {
		panic("value=" + "s3cr3t")
	}}
	_, err := Run(p, []Rule{boom, secretPanic, Lookup1(t, "EG006")})
	var ie *InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *InternalError", err)
	}
	msg := err.Error()
	for _, want := range []string{"EG900 (boom)", "index out of range", "EG901 (secret-panic)", "panic of type string"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
	if strings.Contains(msg, "s3cr3t") {
		t.Error("panic value leaked into the error")
	}
}

func Lookup1(t *testing.T, id string) Rule {
	t.Helper()
	r, ok := Lookup(id)
	if !ok {
		t.Fatalf("rule %s not registered", id)
	}
	return r
}

func TestRunRejectsFindingsWithoutWhyOrFix(t *testing.T) {
	p := loadProject(t, filepath.Join(fixturesDir, "EG006"), nil)
	lazy := fakeRule{base{"EG902", "lazy", findings.Warning}, func(*project.Project) []findings.Finding {
		return []findings.Finding{{Message: "something", File: ".env"}}
	}}
	_, err := Run(p, []Rule{lazy})
	if err == nil || !strings.Contains(err.Error(), "EG902") || !strings.Contains(err.Error(), "Why or Fix") {
		t.Errorf("err = %v", err)
	}
}

func TestRunOverridesAndIgnores(t *testing.T) {
	root := filepath.Join(fixturesDir, "EG006")
	base := loadProject(t, root, nil)
	before, err := Run(base, All())
	if err != nil {
		t.Fatal(err)
	}

	p := loadProject(t, root, func(s *config.Settings) {
		s.Rules = map[string]string{"EG006": "error", "EG002": "off"}
		s.Ignore = []config.IgnoreEntry{{Rule: "EG006", Path: "positive/.env.example"}}
	})
	res, err := Run(p, All())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range res.Rules {
		if id == "EG002" {
			t.Error("EG002 is off but ran")
		}
	}
	var eg006 int
	for _, f := range res.Findings {
		if f.RuleID == "EG002" {
			t.Error("finding of disabled rule EG002")
		}
		if f.RuleID == "EG006" {
			eg006++
			if f.Severity != findings.Error {
				t.Errorf("EG006 severity = %v, want error override", f.Severity)
			}
			if f.File == "positive/.env.example" {
				t.Error("ignored path still reported")
			}
		}
	}
	if eg006 == 0 || res.Suppressed != before.Suppressed+1 {
		t.Errorf("eg006=%d suppressed=%d (before %d)", eg006, res.Suppressed, before.Suppressed)
	}
}

func TestEnvFilter(t *testing.T) {
	p := loadProject(t, filepath.Join(fixturesDir, "EG001"), nil, "prod")
	res, err := Run(p, []Rule{Lookup1(t, "EG001")})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].File != "positive/.env.production" {
		t.Errorf("findings = %s", dump(res.Findings, 0))
	}
}

// TestDeterministic runs all rules concurrently many times over every
// fixture and requires byte-identical results.
func TestDeterministic(t *testing.T) {
	p := loadProject(t, fixturesDir, nil)
	var first string
	for i := range 25 {
		res, err := Run(p, All())
		if err != nil {
			t.Fatal(err)
		}
		got := dump(res.Findings, res.Suppressed)
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("run %d differs from run 0", i)
		}
	}
}

var canonicalEnvs = map[string]bool{
	discovery.EnvDefault: true, discovery.EnvDevelopment: true, discovery.EnvTest: true,
	discovery.EnvStaging: true, discovery.EnvProduction: true,
}

// TestNoValuesInMessages guards principle 3: messages, why and fix texts
// must not contain any value from the fixtures.
func TestNoValuesInMessages(t *testing.T) {
	p := loadProject(t, fixturesDir, nil)
	res, err := Run(p, All())
	if err != nil {
		t.Fatal(err)
	}
	var values []string
	for _, f := range p.EnvFiles {
		for _, e := range f.Entries() {
			// Short values like "1" or "false" and environment names such
			// as "development" legitimately appear in messages.
			if len(e.Value) >= 6 && !canonicalEnvs[e.Value] {
				values = append(values, e.Value)
			}
		}
	}
	for _, f := range res.Findings {
		for _, text := range []string{f.Message, f.Why, f.Fix} {
			for _, v := range values {
				if strings.Contains(text, v) {
					t.Errorf("%s %s: text contains value %q: %s", f.RuleID, f.Location(), v, text)
				}
			}
		}
	}
}

func TestOptionalComment(t *testing.T) {
	for c, want := range map[string]bool{
		"optional": true, "Optional": true, " optional ": true, "optional: seconds": true,
		"optional - used in dev": true, "optionally set": false, "not optional": false, "": false,
	} {
		if got := isOptionalComment(c); got != want {
			t.Errorf("isOptionalComment(%q) = %v", c, got)
		}
	}
}
