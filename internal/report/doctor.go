package report

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/mask"
)

const (
	doctorIndent = "     " // findings body indent (after "  1  ")
	labelWidth   = 9       // "Related  "
)

// Doctor writes the friendly diagnosis report: what was scanned, every
// finding with why it matters and numbered fix steps, the environment
// matrix, the CI inventory, notes and next steps. Without findings it
// prints a short "all good" message with the stats.
func Doctor(w io.Writer, rep *Report, opts Options) error {
	rep = rep.printable()
	s := newStyler(opts)
	var b strings.Builder
	width := opts.width()

	root := rep.Root
	if root == "" {
		root = "."
	}
	b.WriteString(s.paint(s.bold, "EnvGuard doctor") + s.sep() + root)
	if rep.App != "" && rep.App != "." {
		b.WriteString(s.sep() + "app " + rep.App)
	}
	if len(rep.Environments) > 0 {
		b.WriteString(s.sep() + "env " + strings.Join(rep.Environments, ", "))
	}
	b.WriteString("\n\n")

	if len(rep.Findings) == 0 {
		writeAllGood(&b, s, rep)
		_, err := io.WriteString(w, opts.finish(trimRight(b.String())))
		return err
	}

	writeScanned(&b, s, rep)
	writeFindings(&b, s, rep, width)
	writeDoctorMatrix(&b, s, rep)
	writeCI(&b, s, rep)
	writeNotes(&b, s, rep, width)
	writeNextSteps(&b, s, rep, width)
	_, err := io.WriteString(w, opts.finish(trimRight(b.String())))
	return err
}

func heading(b *strings.Builder, s *styler, title, extra string) {
	b.WriteString(s.paint(s.heading, title))
	if extra != "" {
		b.WriteString("  " + extra)
	}
	b.WriteString("\n")
}

func statLine(b *strings.Builder, s *styler, label, value string) {
	b.WriteString("  " + padRight(label, s.paint(s.dim, label), 13) + value + "\n")
}

func writeScanned(b *strings.Builder, s *styler, rep *Report) {
	st := rep.Stats
	heading(b, s, "Scanned", "")
	if rep.ProcessEnv {
		statLine(b, s, "Checked", "the process environment")
	}
	statLine(b, s, "Env files", fmt.Sprintf("%s%s%s", plural(st.EnvFiles, "real", "real"), s.sep(),
		plural(st.Templates, "template", "templates")))
	statLine(b, s, "Compose", plural(st.ComposeFiles, "file", "files"))
	statLine(b, s, "Workflows", plural(st.Workflows, "file", "files"))
	src := plural(st.SourceFiles, "file", "files")
	for _, l := range st.Languages {
		src += s.sep() + fmt.Sprintf("%s %d", l.Language, l.Files)
	}
	statLine(b, s, "Source code", src)
	statLine(b, s, "Variables", fmt.Sprint(st.Variables))
	statLine(b, s, "App roots", fmt.Sprintf("%d%s%s", len(st.AppRoots), s.sep(), strings.Join(st.AppRoots, ", ")))
	cfg := "none (zero-config defaults)"
	if rep.HasConfig {
		cfg = ".envguard.yaml"
		if rep.HasSchema {
			cfg += " with a variable schema"
		}
	}
	statLine(b, s, "Config", cfg)
	git := s.paint(s.green, s.sym.OK) + " available"
	if !rep.GitAvailable {
		git = "not available"
		if rep.GitReason != "" {
			git += " (" + rep.GitReason + ")"
		}
	}
	statLine(b, s, "Git", git)
	b.WriteString("\n")
}

func writeFindings(b *strings.Builder, s *styler, rep *Report, width int) {
	heading(b, s, "Findings", s.counts(findings.Count(rep.Findings)))
	fs := bySeverity(rep.Findings)
	numW := len(fmt.Sprint(len(fs)))
	bodyIndent := strings.Repeat(" ", numW+4)
	textIndent := bodyIndent + strings.Repeat(" ", labelWidth)
	textWidth := max(30, width-len(textIndent))
	for i := range fs {
		f := &fs[i]
		b.WriteString("\n")
		num := fmt.Sprintf("%*d", numW, i+1)
		b.WriteString("  " + s.paint(s.dim, num) + "  ")
		b.WriteString(s.paint(s.sevStyle(f.Severity), s.sevSymbol(f.Severity)) + " ")
		msg := wrap(oneLine(f.Message), max(30, width-numW-6), bodyIndent+"  ")
		b.WriteString(paintLines(msg, func(l string) string {
			return s.highlight(l, f.Variable)
		}) + "\n")
		sub := s.paint(s.sevStyle(f.Severity), f.Severity.String()) + s.sep() + f.RuleID + " " + f.RuleName +
			s.sep() + f.Location().String()
		b.WriteString(bodyIndent + "  " + sub + "\n\n")

		label := func(name string) string {
			return bodyIndent + padRight(name, s.paint(s.bold, name), labelWidth)
		}
		b.WriteString(label("Why") + paintLines(wrap(f.Why, textWidth, textIndent), s.code) + "\n")
		steps := fixSteps(f.Fix)
		for j, st := range steps {
			prefix := label("Fix")
			if j > 0 {
				prefix = textIndent
			}
			stepIndent := textIndent
			if len(steps) > 1 {
				n := fmt.Sprintf("%d. ", j+1)
				prefix += n
				stepIndent += strings.Repeat(" ", len(n))
			}
			if st.Text != "" {
				b.WriteString(prefix + paintLines(wrap(st.Text, max(20, width-len(stepIndent)), stepIndent), s.code) + "\n")
			} else {
				b.WriteString(prefix + "\n")
			}
			for _, c := range st.Code {
				if strings.TrimSpace(c) == "" {
					b.WriteString("\n")
					continue
				}
				b.WriteString(stepIndent + "  " + s.paint(s.codeStyle, c) + "\n")
			}
		}
		if f.Evidence != "" {
			b.WriteString(label("Evidence") + oneLine(f.Evidence) + "\n")
		}
		for j, r := range f.Related {
			prefix := label("Related")
			if j > 0 {
				prefix = textIndent
			}
			b.WriteString(prefix + s.paint(s.dim, r.String()) + "\n")
		}
	}
	b.WriteString("\n")
}

// paintLines applies fn to every line of text.
func paintLines(text string, fn func(string) string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = fn(l)
	}
	return strings.Join(lines, "\n")
}

func writeDoctorMatrix(b *strings.Builder, s *styler, rep *Report) {
	if len(rep.Matrix) == 0 {
		return
	}
	heading(b, s, "Environment matrix", "")
	for i := range rep.Matrix {
		if i > 0 {
			b.WriteString("\n")
		}
		writeMatrix(b, s, &rep.Matrix[i], "  ")
	}
	b.WriteString("  " + legend(s) + "\n\n")
}

func writeCI(b *strings.Builder, s *styler, rep *Report) {
	if rep.Stats.Workflows == 0 {
		return
	}
	heading(b, s, "CI inventory", s.paint(s.dim, "what GitHub Actions workflows expect"))
	if len(rep.CI.Secrets)+len(rep.CI.Vars) == 0 {
		b.WriteString("  The workflows reference no secrets or vars.\n\n")
		return
	}
	nameW := 0
	for _, it := range append(append([]CIItem{}, rep.CI.Secrets...), rep.CI.Vars...) {
		nameW = max(nameW, min(utf8.RuneCountInString(it.Name), maxNameWidth))
	}
	group := func(label string, items []CIItem) {
		for i, it := range items {
			l := ""
			if i == 0 {
				l = label
			}
			where := ""
			if len(it.Refs) > 0 {
				where = it.Refs[0].String()
				if len(it.Refs) > 1 {
					where += fmt.Sprintf(" (+%d more)", len(it.Refs)-1)
				}
			}
			name := mask.Clip(it.Name, maxNameWidth)
			b.WriteString("  " + padRight(l, s.paint(s.dim, l), 9) + padRight(name, name, nameW) + "  " +
				s.paint(s.dim, where) + "\n")
		}
	}
	group("Secrets", rep.CI.Secrets)
	group("Vars", rep.CI.Vars)
	b.WriteString("\n")
}

func writeNotes(b *strings.Builder, s *styler, rep *Report, width int) {
	var notes []string
	if n := rep.Stats.DynamicLookups; n > 0 {
		notes = append(notes, fmt.Sprintf("%s (like `process.env[name]`) could not be analyzed; variables read that way are not checked.",
			plural(n, "dynamic env lookup", "dynamic env lookups")))
	}
	if rep.Suppressed > 0 {
		notes = append(notes, fmt.Sprintf("%s hidden by .envguard.yaml ignores or `envguard-ignore` directives.",
			plural(rep.Suppressed, "finding is", "findings are")))
	}
	if !rep.GitAvailable && !rep.ProcessEnv {
		notes = append(notes, "Git checks (EG008, EG009) were skipped: run EnvGuard inside a git work tree with git installed.")
	}
	const maxNotes = 5
	for i, n := range rep.Notes {
		if i == maxNotes {
			notes = append(notes, fmt.Sprintf("%d more framework notes not shown (see --format json).", len(rep.Notes)-maxNotes))
			break
		}
		notes = append(notes, fmt.Sprintf("%s: %s.", n.String(), n.Text))
	}
	if len(notes) == 0 {
		return
	}
	heading(b, s, "Good to know", "")
	for _, n := range notes {
		b.WriteString("  " + s.paint(s.dim, s.sym.Bullet) + " " + paintLines(wrap(n, width-4, "    "), s.code) + "\n")
	}
	b.WriteString("\n")
}

func writeNextSteps(b *strings.Builder, s *styler, rep *Report, width int) {
	c := findings.Count(rep.Findings)
	var steps []string
	top := bySeverity(rep.Findings)[0]
	switch {
	case c.Error > 0:
		steps = append(steps, fmt.Sprintf("Fix the %s first: they break deploys or expose secrets.",
			plural(c.Error, "error", "errors")))
	case c.Warning > 0:
		steps = append(steps, fmt.Sprintf("Review the %s; each one explains why it matters.",
			plural(c.Warning, "warning", "warnings")))
	default:
		steps = append(steps, "Only informational findings left: clean them up when convenient.")
	}
	steps = append(steps, fmt.Sprintf("Learn more about a rule: `envguard explain %s` (all rules: `envguard explain --list`).", top.RuleID))
	if !rep.HasConfig {
		steps = append(steps, "Run `envguard init` to generate .envguard.yaml with types and required flags for every variable.")
	} else if !rep.HasSchema {
		steps = append(steps, "Describe your variables under `variables:` in .envguard.yaml to enable type and production checks.")
	}
	steps = append(steps, "Run `envguard check` in CI: it exits 1 on errors and annotates pull requests in GitHub Actions.")
	heading(b, s, "Next steps", "")
	for i, st := range steps {
		n := fmt.Sprintf("%d. ", i+1)
		b.WriteString("  " + n + paintLines(wrap(st, width-5, "     "), s.code) + "\n")
	}
}

func writeAllGood(b *strings.Builder, s *styler, rep *Report) {
	st := rep.Stats
	b.WriteString(s.paint(s.green, s.sym.OK+" All good") + " " + s.sym.Dash + " no problems found.\n\n")
	if rep.ProcessEnv {
		b.WriteString("  The process environment matches the templates and schema.\n")
	}
	parts := []string{
		plural(st.EnvFiles, "env file", "env files"),
		plural(st.Templates, "template", "templates"),
		plural(st.ComposeFiles, "Compose file", "Compose files"),
		plural(st.Workflows, "workflow", "workflows"),
		plural(st.SourceFiles, "source file", "source files"),
	}
	b.WriteString("  " + strings.Join(parts, s.sep()) + "\n")
	second := []string{plural(st.Variables, "variable", "variables"), plural(len(st.AppRoots), "app root", "app roots")}
	if rep.Suppressed > 0 {
		second = append(second, fmt.Sprintf("%d suppressed", rep.Suppressed))
	}
	if st.DynamicLookups > 0 {
		second = append(second, plural(st.DynamicLookups, "dynamic lookup", "dynamic lookups")+" not analyzed")
	}
	b.WriteString("  " + strings.Join(second, s.sep()) + "\n\n")
	next := "Keep it that way: run `envguard check` in CI."
	if !rep.HasConfig {
		next = "Next: `envguard init` adds a schema with types, and `envguard check` in CI keeps it this way."
	}
	b.WriteString(s.code(next) + "\n")
}
