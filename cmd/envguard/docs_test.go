package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/report"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

// Documentation is tested so it can never drift from behavior. Markdown files
// mark code blocks with an HTML comment on the line before the fence:
//
//	<!-- envguard-example -->            the block is `$ envguard ARGS` plus the
//	<!-- envguard-example excerpt -->    exact output (or, with excerpt, lines that
//	<!-- envguard-example exit=1 -->     appear in this order; "..." skips). ARGS run
//	                                     against examples/demo-app (as --root) in a
//	                                     git repository where every file is committed.
//	<!-- envguard-file path=src/a.js --> the block's lines appear, in order, in
//	                                     that file of examples/demo-app.
//	<!-- envguard-config -->             the yaml block is a valid .envguard.yaml.
//
// The rules table in README.md between <!-- rules-table:start --> and
// <!-- rules-table:end --> is generated; refresh it with
// go test ./cmd/envguard -run TestReadmeRulesTable -update.

var (
	repoRoot  = filepath.Join("..", "..")
	markerRE  = regexp.MustCompile(`^<!--\s*envguard-(example|file|config)\b(.*?)-->\s*$`)
	optionRE  = regexp.MustCompile(`(\w+)(?:=(\S+))?`)
	elisionRE = regexp.MustCompile(`^\s*(\.{3}|…)\s*$`)
)

type docBlock struct {
	file, kind string
	line       int
	opts       map[string]string
	body       []string
}

func (b docBlock) String() string { return fmt.Sprintf("%s:%d (envguard-%s)", b.file, b.line, b.kind) }

func docFiles(t *testing.T) []string {
	t.Helper()
	files := []string{filepath.Join(repoRoot, "README.md"), filepath.Join(repoRoot, "CONTRIBUTING.md")}
	err := filepath.WalkDir(filepath.Join(repoRoot, "docs"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(files, func(p string) bool { _, err := os.Stat(p); return err != nil })
}

func parseDocBlocks(t *testing.T, file string) []docBlock {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(repoRoot, file)
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var out []docBlock
	for i := 0; i < len(lines); i++ {
		m := markerRE.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		b := docBlock{file: filepath.ToSlash(rel), kind: m[1], line: i + 1, opts: map[string]string{}}
		for _, o := range optionRE.FindAllStringSubmatch(m[2], -1) {
			b.opts[o[1]] = o[2]
		}
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
			j++
		}
		if j >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
			t.Errorf("%s: marker is not followed by a fenced code block", b)
			continue
		}
		indent, _, _ := strings.Cut(lines[j], "```")
		for j++; j < len(lines) && strings.TrimSpace(lines[j]) != "```"; j++ {
			b.body = append(b.body, strings.TrimRight(strings.TrimPrefix(lines[j], indent), " \t"))
		}
		out = append(out, b)
		i = j
	}
	return out
}

// demoRepo is a copy of examples/demo-app in which every file is committed,
// as in a clone of this repository.
func demoRepo(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixture(t, filepath.Join(repoRoot, "examples", "demo-app"))
	var all []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			all = append(all, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	testutil.InitGit(t, dir, all)
	return dir
}

// splitArgs splits a command line on blanks, honoring "double" and 'single'
// quotes (enough for documentation examples).
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// subsequence reports the first documented line that is not found, in
// order, among got ("" if all are). Elision lines are skipped.
func subsequence(want, got []string) string {
	k := 0
	for _, w := range want {
		if elisionRE.MatchString(w) {
			continue
		}
		for k < len(got) && got[k] != w {
			k++
		}
		if k == len(got) {
			return w
		}
		k++
	}
	return ""
}

func trimLines(s string) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return lines
}

func TestDocExamples(t *testing.T) {
	var blocks []docBlock
	for _, f := range docFiles(t) {
		blocks = append(blocks, parseDocBlocks(t, f)...)
	}
	if !slices.ContainsFunc(blocks, func(b docBlock) bool { return b.kind == "example" && b.file == "README.md" }) {
		t.Fatal("README.md has no envguard-example blocks; the docs test checks nothing")
	}
	demo := demoRepo(t)
	demoSrc := filepath.Join(repoRoot, "examples", "demo-app")

	for _, b := range blocks {
		t.Run(strings.NewReplacer("/", "_", ":", "_", " ", "_").Replace(b.String()), func(t *testing.T) {
			switch b.kind {
			case "example":
				checkExample(t, b, demo)
			case "file":
				p := b.opts["path"]
				if p == "" {
					t.Fatalf("%s: envguard-file needs path=", b)
				}
				data, err := os.ReadFile(filepath.Join(demoSrc, filepath.FromSlash(p)))
				if err != nil {
					t.Fatalf("%s: %v", b, err)
				}
				if miss := subsequence(b.body, trimLines(string(data))); miss != "" {
					t.Errorf("%s: line %q is not in examples/demo-app/%s (in this order)", b, miss, p)
				}
			case "config":
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, ".envguard.yaml"), []byte(strings.Join(b.body, "\n")+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := config.Load(config.Flags{Root: dir}); err != nil {
					t.Errorf("%s: invalid configuration: %v", b, err)
				}
			}
		})
	}
}

func checkExample(t *testing.T, b docBlock, demo string) {
	t.Helper()
	if len(b.body) == 0 || !strings.HasPrefix(b.body[0], "$ envguard") {
		t.Fatalf("%s: the first line must be `$ envguard ...`", b)
	}
	args := splitArgs(strings.TrimPrefix(b.body[0], "$ envguard"))
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"--root", demo}, args...), &stdout, &stderr)
	got := trimLines(stdout.String() + stderr.String())
	want := b.body[1:]
	if e, ok := b.opts["exit"]; ok {
		if n, _ := strconv.Atoi(e); n != code {
			t.Errorf("%s: exit code %d, documented %s", b, code, e)
		}
	}
	if _, excerpt := b.opts["excerpt"]; excerpt {
		if miss := subsequence(want, got); miss != "" {
			t.Errorf("%s: documented line not in the output (in this order):\n  %q\nactual output:\n%s", b, miss, strings.Join(got, "\n"))
		}
		return
	}
	for len(want) > 0 && want[len(want)-1] == "" {
		want = want[:len(want)-1]
	}
	if strings.Join(want, "\n") != strings.Join(got, "\n") {
		t.Errorf("%s: documented output differs from the real output.\nreal output:\n%s", b, strings.Join(got, "\n"))
	}
}

// TestReadmeRulesTable keeps the README rules table in sync with the
// registered rules and their docs.
func TestReadmeRulesTable(t *testing.T) {
	path := filepath.Join(repoRoot, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const start, end = "<!-- rules-table:start -->", "<!-- rules-table:end -->"
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	i, j := strings.Index(s, start), strings.Index(s, end)
	if i < 0 || j < i {
		t.Fatalf("README.md lacks %s ... %s", start, end)
	}
	var tb strings.Builder
	tb.WriteString("\n| Rule | Name | Severity | Catches |\n|---|---|---|---|\n")
	for _, r := range report.RuleInfos() {
		file := r.ID + "-" + r.Name + ".md"
		fmt.Fprintf(&tb, "| [%s](docs/rules/%s) | `%s` | %s | %s |\n", r.ID, file, r.Name, r.Severity, r.Summary)
	}
	want := s[:i+len(start)] + tb.String() + s[j:]
	if testutil.Updating() {
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if want != s {
		t.Error("the README rules table is out of date; run: go test ./cmd/envguard -run TestReadmeRulesTable -update")
	}
}

var mdLinkRE = regexp.MustCompile(`\]\(([^)#\s]+)(#[^)\s]*)?\)`)

// TestDocLinks checks that relative links in the documentation point to
// files that exist.
func TestDocLinks(t *testing.T) {
	files := docFiles(t)
	for _, extra := range []string{"SECURITY.md", "CHANGELOG.md", filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md")} {
		if _, err := os.Stat(filepath.Join(repoRoot, extra)); err == nil {
			files = append(files, filepath.Join(repoRoot, extra))
		}
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		inFence := false
		for n, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
			}
			if inFence {
				continue
			}
			for _, m := range mdLinkRE.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
					continue
				}
				p := filepath.Join(filepath.Dir(f), filepath.FromSlash(target))
				if _, err := os.Stat(p); err != nil {
					rel, _ := filepath.Rel(repoRoot, f)
					t.Errorf("%s:%d: broken link %s", filepath.ToSlash(rel), n+1, target)
				}
			}
		}
	}
}

var (
	tapeSleepRE  = regexp.MustCompile(`(?m)^Sleep\s+(\d+(?:\.\d+)?)(ms|s)\s*$`)
	tapeTypeRE   = regexp.MustCompile(`(?m)^Type\s+"([^"]*)"\s*$`)
	tapeSpeedRE  = regexp.MustCompile(`(?m)^Set TypingSpeed\s+(\d+)ms\s*$`)
	tapeHideRE   = regexp.MustCompile(`(?s)\nHide\n.*?\nShow\n`)
	tapeOutputRE = regexp.MustCompile(`(?m)^Output\s+(\S+)\s*$`)
)

// TestDemoTape keeps docs/demo.tape honest: it records the four documented
// commands in order, finishes in under 20 seconds, and writes docs/demo.gif.
func TestDemoTape(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "demo.tape"))
	if err != nil {
		t.Fatal(err)
	}
	tape := string(data)
	if m := tapeOutputRE.FindStringSubmatch(tape); len(m) < 2 || m[1] != "docs/demo.gif" {
		t.Errorf("the tape must write docs/demo.gif, got %v", m)
	}

	// Commands the viewer sees: Type lines outside Hide ... Show.
	visible := tapeHideRE.ReplaceAllString("\n"+tape, "\n")
	var shown []string
	typed := 0
	for _, m := range tapeTypeRE.FindAllStringSubmatch(visible, -1) {
		shown = append(shown, m[1])
		typed += len(m[1])
	}
	want := []string{"envguard", "envguard check", "envguard explain EG003", "envguard diff"}
	if len(shown) != len(want) {
		t.Fatalf("the tape types %d visible commands, want %d: %q", len(shown), len(want), shown)
	}
	for i, w := range want {
		if !strings.Contains(shown[i], w) {
			t.Errorf("visible command %d is %q, want it to contain %q", i+1, shown[i], w)
		}
	}

	seconds := 0.0
	for _, m := range tapeSleepRE.FindAllStringSubmatch(tape, -1) {
		v, _ := strconv.ParseFloat(m[1], 64)
		if m[2] == "ms" {
			v /= 1000
		}
		seconds += v
	}
	speed := 45.0
	if m := tapeSpeedRE.FindStringSubmatch(tape); m != nil {
		speed, _ = strconv.ParseFloat(m[1], 64)
	}
	seconds += float64(typed) * speed / 1000
	if seconds >= 20 {
		t.Errorf("the recording takes about %.1fs; keep it under 20s", seconds)
	}

	// Every command in the tape must exist.
	for _, c := range want {
		args := strings.Fields(strings.TrimPrefix(c, "envguard"))
		var out bytes.Buffer
		if code := run(append([]string{"--root", filepath.Join(repoRoot, "examples", "demo-app")}, args...), &out, &out); code > 1 {
			t.Errorf("`%s` exits with %d:\n%s", c, code, out.String())
		}
	}
}
