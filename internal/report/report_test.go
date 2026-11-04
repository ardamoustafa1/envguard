package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ardamoustafa1/envguard/docs"
	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/rules"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

// goldenProjects are analyzed and rendered in every format.
var goldenProjects = []struct{ name, src string }{
	{"demo", filepath.Join("..", "..", "examples", "demo-app")},
	{"compose", filepath.Join("..", "..", "testdata", "fixtures", "EG011")},
	{"ci", filepath.Join("..", "..", "testdata", "fixtures", "EG012")},
	{"schema", filepath.Join("..", "..", "testdata", "fixtures", "EG015")},
	{"clean", filepath.Join("testdata", "projects", "clean")},
}

// analyze copies src to a temp dir (no git) and builds its report.
func analyze(t *testing.T, src string) (*Report, *project.Project) {
	t.Helper()
	dir := testutil.CopyFixture(t, src)
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
	return Build(p, res.Findings, Meta{Command: "check", Version: "test", Root: ".", Suppressed: res.Suppressed,
		Duration: 80 * time.Millisecond}), p
}

func render(t *testing.T, fn func(*bytes.Buffer) error) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := fn(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestGolden(t *testing.T) {
	for _, gp := range goldenProjects {
		t.Run(gp.name, func(t *testing.T) {
			rep, p := analyze(t, gp.src)
			outputs := map[string][]byte{}
			for _, f := range Formats() {
				outputs[f] = render(t, func(b *bytes.Buffer) error { return Write(b, f, rep, Options{}) })
			}
			outputs["pretty-ascii"] = render(t, func(b *bytes.Buffer) error { return Pretty(b, rep, Options{ASCII: true}) })
			outputs["doctor-ascii"] = render(t, func(b *bytes.Buffer) error { return Doctor(b, rep, Options{ASCII: true}) })
			outputs["github-summary"] = render(t, func(b *bytes.Buffer) error { return GitHubSummary(b, rep) })
			outputs["matrix"] = render(t, func(b *bytes.Buffer) error { return MatrixText(b, rep.Matrix, Options{}) })
			if gp.name == "demo" {
				outputs["pretty-color"] = render(t, func(b *bytes.Buffer) error { return Pretty(b, rep, Options{Color: true}) })
				outputs["doctor-color"] = render(t, func(b *bytes.Buffer) error { return Doctor(b, rep, Options{Color: true, Width: 100}) })
				d, err := BuildDiff(p, "", "staging", "prod")
				if err != nil {
					t.Fatal(err)
				}
				outputs["diff"] = render(t, func(b *bytes.Buffer) error { return DiffText(b, d, Options{}) })
				outputs["diff-json"] = render(t, func(b *bytes.Buffer) error { return DiffJSON(b, "test", ".", d) })
				outputs["matrix-json"] = render(t, func(b *bytes.Buffer) error { return MatrixJSON(b, "test", ".", rep.Matrix) })
			}
			for name, out := range outputs {
				testutil.Golden(t, filepath.Join("testdata", "golden", gp.name, name+".golden"), out)
				if !strings.Contains(name, "color") && bytes.Contains(out, []byte("\x1b[")) {
					t.Errorf("%s contains ANSI escapes without Color", name)
				}
				if strings.HasSuffix(name, "-ascii") {
					for i, c := range out {
						if c > 0x7e || (c < 0x20 && c != '\n') {
							t.Errorf("%s: non-ASCII byte at %d: %q", name, i, out[max(0, i-20):min(len(out), i+10)])
							break
						}
					}
				}
			}
			// Same input, same bytes.
			again, _ := analyze(t, gp.src)
			for _, f := range Formats() {
				a := render(t, func(b *bytes.Buffer) error { return Write(b, f, again, Options{}) })
				if !bytes.Equal(a, outputs[f]) {
					t.Errorf("%s output is not deterministic", f)
				}
			}
		})
	}
}

func TestDemoCoversMostRules(t *testing.T) {
	rep, _ := analyze(t, goldenProjects[0].src)
	fired := map[string]bool{}
	for _, f := range rep.Findings {
		fired[f.RuleID] = true
	}
	// EG008 and EG009 need git; the copy has none.
	for _, id := range []string{"EG001", "EG002", "EG003", "EG004", "EG005", "EG006", "EG007", "EG010", "EG011",
		"EG012", "EG013", "EG014", "EG015", "EG016"} {
		if !fired[id] {
			t.Errorf("examples/demo-app does not trigger %s", id)
		}
	}
}

func TestSARIFValidatesAgainstSchema(t *testing.T) {
	const id = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"
	raw, err := os.ReadFile(filepath.Join("testdata", "sarif-schema-2.1.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, gp := range goldenProjects {
		rep, _ := analyze(t, gp.src)
		rep.PathPrefix = "sub dir/"
		out := render(t, func(b *bytes.Buffer) error { return SARIF(b, rep) })
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(inst); err != nil {
			t.Errorf("%s: SARIF output does not validate: %v", gp.name, err)
		}
		var log sarifLog
		if err := json.Unmarshal(out, &log); err != nil {
			t.Fatal(err)
		}
		if got := len(log.Runs[0].Tool.Driver.Rules); got != len(rules.All()) {
			t.Errorf("%s: %d rules in SARIF, want %d", gp.name, got, len(rules.All()))
		}
		fps := map[string]bool{}
		for _, r := range log.Runs[0].Results {
			if r.RuleID != log.Runs[0].Tool.Driver.Rules[r.RuleIndex].ID {
				t.Errorf("ruleIndex of %s points to the wrong rule", r.RuleID)
			}
			uri := r.Locations[0].PhysicalLocation.ArtifactLocation.URI
			if !strings.HasPrefix(uri, "sub%20dir/") {
				t.Errorf("uri %q lacks the escaped path prefix", uri)
			}
			fp := r.PartialFingerprints[fingerprintKey]
			if fps[fp] {
				t.Errorf("%s: duplicate fingerprint %s", gp.name, fp)
			}
			fps[fp] = true
		}
	}
}

func TestFingerprintIgnoresLine(t *testing.T) {
	a := findings.Finding{RuleID: "EG001", File: ".env", Variable: "X", Line: 3, Message: "m"}
	b := a
	b.Line = 40
	if fingerprint(fingerprintBase(&a), 0) != fingerprint(fingerprintBase(&b), 0) {
		t.Error("fingerprint changes with the line number")
	}
	if fingerprint(fingerprintBase(&a), 0) == fingerprint(fingerprintBase(&a), 1) {
		t.Error("occurrences share a fingerprint")
	}
}

func TestGitHubEscaping(t *testing.T) {
	rep := &Report{Findings: []findings.Finding{{
		RuleID: "EG007", RuleName: "syntax-error", Severity: findings.Error, File: "a,b:c%.env", Line: 2, Column: 3,
		Message: "50% broken\nline two\r", Why: "w", Fix: "f",
	}, {
		RuleID: "EG004", RuleName: "declared-but-unused", Severity: findings.Info, File: ".env.example", Message: "m", Why: "w", Fix: "f",
	}}, PathPrefix: "apps/api/"}
	out := string(render(t, func(b *bytes.Buffer) error { return GitHub(b, rep) }))
	want := "::error file=apps/api/a%2Cb%3Ac%25.env,line=2,col=3,title=EG007 syntax-error::50%25 broken%0Aline two\\x0d%0A%0AWhy: w%0A%0AFix: f\n" +
		"::notice file=apps/api/.env.example,title=EG004 declared-but-unused::m%0A%0AWhy: w%0A%0AFix: f\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("got\n%s\nwant prefix\n%s", out, want)
	}
	sum := string(render(t, func(b *bytes.Buffer) error { return GitHubSummary(b, rep) }))
	if !strings.Contains(sum, "`apps/api/a,b:c%.env:2:3`") || !strings.Contains(sum, "50% broken line two") {
		t.Errorf("summary:\n%s", sum)
	}
	if got := mdCell("a|b<script>&"); got != `a\|b&lt;script&gt;&amp;` {
		t.Errorf("mdCell = %q", got)
	}
}

func TestPathPrefix(t *testing.T) {
	for in, want := range map[string]string{".": "", "": "", "./apps/api/": "apps/api/", "examples/demo-app": "examples/demo-app/",
		"/abs/path": "", "../up": "", `C:\x`: "", `apps\api`: "apps/api/"} {
		if got := pathPrefix(in); got != want {
			t.Errorf("pathPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrettyDuration(t *testing.T) {
	rep := &Report{Findings: []findings.Finding{}, Duration: 83 * time.Millisecond}
	with := string(render(t, func(b *bytes.Buffer) error { return Pretty(b, rep, Options{ShowDuration: true}) }))
	without := string(render(t, func(b *bytes.Buffer) error { return Pretty(b, rep, Options{}) }))
	if !strings.Contains(with, "0.08s") || strings.Contains(without, "s\n") && strings.Contains(without, "0.0") {
		t.Errorf("with: %q, without: %q", with, without)
	}
	if without != "✓ No problems found · 0 files\n" {
		t.Errorf("without = %q", without)
	}
}

func TestFixSteps(t *testing.T) {
	tests := []struct {
		fix  string
		want []step
	}{
		{"Do one thing.", []step{{Text: "Do one thing."}}},
		{"Add `A=` to .env (declared in .env.example:4) and set it. If it is not needed, mark it `A= # optional. Really`.",
			[]step{{Text: "Add `A=` to .env (declared in .env.example:4) and set it."}, {Text: "If it is not needed, mark it `A= # optional. Really`."}}},
		{"1. First `git rm`\n2. Second\n   continued.\n3. Third", []step{{Text: "First `git rm`"}, {Text: "Second continued."}, {Text: "Third"}}},
		{"Create it, or use:\n\n    env_file:\n      - path: x", []step{{Text: "Create it, or use:", Code: []string{"env_file:", "  - path: x"}}}},
	}
	for _, tt := range tests {
		got := fixSteps(tt.fix)
		if len(got) != len(tt.want) {
			t.Errorf("fixSteps(%q) = %#v", tt.fix, got)
			continue
		}
		for i := range got {
			if got[i].Text != tt.want[i].Text || strings.Join(got[i].Code, "\n") != strings.Join(tt.want[i].Code, "\n") {
				t.Errorf("fixSteps(%q)[%d] = %#v, want %#v", tt.fix, i, got[i], tt.want[i])
			}
		}
	}
}

func TestWrapKeepsCodeSpans(t *testing.T) {
	got := wrap("aaaa bbbb `c d e f g h i j` kkkk", 20, "  ")
	if got != "aaaa bbbb\n  `c d e f g h i j`\n  kkkk" {
		t.Errorf("wrap = %q", got)
	}
}

func TestExplainRendering(t *testing.T) {
	d, ok := docs.Rule("EG001")
	if !ok {
		t.Fatal("no doc for EG001")
	}
	testutil.Golden(t, filepath.Join("testdata", "golden", "explain-EG001.golden"),
		render(t, func(b *bytes.Buffer) error { return Markdown(b, d.Markdown, Options{}) }))
	testutil.Golden(t, filepath.Join("testdata", "golden", "explain-list.golden"),
		render(t, func(b *bytes.Buffer) error { return RuleList(b, RuleInfos(), Options{Width: 100}) }))
}

// TestEveryRuleHasDocs fails if a registered rule lacks a complete document.
func TestEveryRuleHasDocs(t *testing.T) {
	sections := []string{"What it detects", "Why it matters", "Bad example", "Good example", "How to fix", "How to suppress"}
	for _, r := range rules.All() {
		d, ok := docs.Rule(r.ID())
		if !ok {
			t.Errorf("%s has no docs/rules/%s-%s.md", r.ID(), r.ID(), r.Name())
			continue
		}
		if d.File != r.ID()+"-"+r.Name()+".md" || d.Name != r.Name() {
			t.Errorf("%s: doc file %s / title name %q do not match the rule name %s", r.ID(), d.File, d.Name, r.Name())
		}
		if !strings.HasPrefix(d.Severity, r.DefaultSeverity().String()) {
			t.Errorf("%s: doc severity %q, rule default %s", r.ID(), d.Severity, r.DefaultSeverity())
		}
		if d.Summary == "" || !strings.HasSuffix(d.Summary, ".") {
			t.Errorf("%s: missing one-sentence summary", r.ID())
		}
		for _, s := range sections {
			if d.Section(s) == "" {
				t.Errorf("%s: doc lacks a %q section", r.ID(), s)
			}
		}
	}
	if len(docs.AllRules()) != len(rules.All()) {
		t.Errorf("%d rule docs for %d rules", len(docs.AllRules()), len(rules.All()))
	}
}
