package report

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

func hostileReport() *Report {
	evil := ".env.\x1b[2J\x1b]0;pwned\x07prod"
	bidi := "src/evil\u202etxt.js"
	return &Report{
		Tool: Tool{Name: "envguard", Version: "test"}, Command: "check", Root: "re\x1bpo",
		Findings: []findings.Finding{{
			RuleID: "EG002", RuleName: "undocumented-variable", Severity: findings.Warning,
			Message: "X is set in " + evil + " but not documented", Variable: "X", File: evil, Line: 1, Column: 1,
			Why: "Why \x1b[31m" + bidi, Fix: "1. Fix " + evil + "\n\n    code " + bidi, Evidence: "ev\x1b",
			Related: []findings.Location{{File: bidi, Line: 2}}, DocsURL: "https://example.invalid/x",
		}},
		Matrix: []AppMatrix{{Root: "app\x1b", Environments: []string{evil}, TemplateOnly: []string{},
			Files:     map[string][]string{evil: {evil}},
			Variables: []MatrixRow{{Name: "N\x1b", Cells: map[string]CellState{evil: CellSet}}}}},
		CI:    CIInventory{Secrets: []CIItem{{Name: "S\x1b", Refs: []findings.Location{{File: bidi, Line: 3}}}}},
		Notes: []Note{{Location: findings.Location{File: bidi, Line: 1}, Variable: "V\x1b", Text: "note \x1b"}},
		Stats: Stats{AppRoots: []string{".", "a\x1bb"}, Languages: []LanguageCount{{Language: "go\x1b", Files: 1}}},
	}
}

func hasTerminalControl(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}

// File names, variable names and environment names come from the analyzed
// repository. No text renderer may pass terminal control sequences through.
func TestTextRenderersEscapeHostileNames(t *testing.T) {
	rep := hostileReport()
	for _, ascii := range []bool{false, true} {
		for name, render := range map[string]func(*bytes.Buffer) error{
			"pretty":         func(b *bytes.Buffer) error { return Pretty(b, rep, Options{ASCII: ascii}) },
			"doctor":         func(b *bytes.Buffer) error { return Doctor(b, rep, Options{ASCII: ascii}) },
			"github":         func(b *bytes.Buffer) error { return GitHub(b, rep) },
			"github-summary": func(b *bytes.Buffer) error { return GitHubSummary(b, rep) },
			"matrix":         func(b *bytes.Buffer) error { return MatrixText(b, rep.Matrix, Options{ASCII: ascii}) },
			"diff": func(b *bytes.Buffer) error {
				return DiffText(b, &EnvDiff{A: "a\x1b", B: "b", Apps: []AppDiff{{Root: "r\x1b", AFiles: []string{"f\x1b"},
					OnlyInA: []string{"K\x1b"}, OnlyInB: []string{}}}}, Options{ASCII: ascii})
			},
		} {
			var b bytes.Buffer
			if err := render(&b); err != nil {
				t.Fatal(err)
			}
			if hasTerminalControl(b.String()) {
				t.Errorf("%s (ascii=%v) passes a control character through:\n%q", name, ascii, b.String())
			}
			if !strings.Contains(b.String(), `\x1b`) {
				t.Errorf("%s (ascii=%v) does not show the escaped name", name, ascii)
			}
		}
	}
	// JSON and SARIF keep the raw names, but their encoders escape them.
	for name, render := range map[string]func(*bytes.Buffer) error{
		"json":  func(b *bytes.Buffer) error { return JSON(b, rep) },
		"sarif": func(b *bytes.Buffer) error { return SARIF(b, rep) },
	} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		if strings.ContainsRune(b.String(), 0x1b) {
			t.Errorf("%s output contains a raw ESC byte", name)
		}
	}
}

// Found by running a repository with one 500,000-character variable name:
// every matrix row was padded to the longest name (tens of gigabytes).
func TestAlignedOutputIsBoundedByLongNames(t *testing.T) {
	long := strings.Repeat("K", 500_000)
	rows := make([]MatrixRow, 2000)
	for i := range rows {
		rows[i] = MatrixRow{Name: "VAR_" + strconv.Itoa(i), Cells: map[string]CellState{"default": CellSet}}
	}
	rows[0].Name = long
	ms := []AppMatrix{{Root: ".", Environments: []string{"default", long[:300]}, Variables: rows}}
	var b bytes.Buffer
	if err := MatrixText(&b, ms, Options{}); err != nil {
		t.Fatal(err)
	}
	if b.Len() > 2000*(maxNameWidth+maxEnvWidth*2+20) {
		t.Errorf("matrix output is %d bytes for 2000 rows; column widths are not capped", b.Len())
	}
	if !strings.Contains(b.String(), strings.Repeat("K", maxNameWidth-1)+"…") {
		t.Error("the long name is not clipped with an ellipsis")
	}
	d := &EnvDiff{A: "a", B: "b", Apps: []AppDiff{{Root: ".", OnlyInA: []string{long, "x"}, OnlyInB: []string{"y"}}}}
	b.Reset()
	if err := DiffText(&b, d, Options{}); err != nil {
		t.Fatal(err)
	}
	if b.Len() > 4096 {
		t.Errorf("diff output is %d bytes for three short rows", b.Len())
	}
}
