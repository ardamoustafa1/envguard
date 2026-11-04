package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/mask"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/rules"
)

// CellState says whether an environment defines a variable. It never
// carries the value.
type CellState string

// Cell states.
const (
	// CellSet: a real env file of the environment sets a non-empty value.
	CellSet CellState = "set"
	// CellEmpty: defined, but the (last) value is empty.
	CellEmpty CellState = "empty"
	// CellMissing: a template that applies to the environment declares the
	// variable as required (not `# optional`), but no real env file of the
	// environment defines it.
	CellMissing CellState = "missing"
	// CellAbsent: not defined, and not required for the environment (not
	// declared, or declared `# optional`).
	CellAbsent CellState = "absent"
)

// AppMatrix is the variables × environments matrix of one app root.
// Columns are the environments that have a real env file (or the process
// environment); environments with only templates are listed separately.
type AppMatrix struct {
	Root         string              `json:"app_root"`
	Environments []string            `json:"environments"`
	TemplateOnly []string            `json:"template_only_environments"`
	Files        map[string][]string `json:"files"`
	Variables    []MatrixRow         `json:"variables"`
}

// MatrixRow is one variable.
type MatrixRow struct {
	Name  string               `json:"name"`
	Cells map[string]CellState `json:"cells"`
}

// BuildMatrix computes the environment matrix of every app root (or only of
// app, if set), honoring the --env filter.
func BuildMatrix(p *project.Project, app string) []AppMatrix {
	out := []AppMatrix{}
	for _, a := range p.Apps {
		if app != "" && app != "." && a.Root != app {
			continue
		}
		m := AppMatrix{Root: a.Root, Environments: []string{}, TemplateOnly: []string{},
			Files: map[string][]string{}, Variables: []MatrixRow{}}
		rows := map[string]bool{}
		defined := map[string]map[string]CellState{}
		declared := map[string]map[string]bool{}
		for _, g := range a.Groups {
			if !p.EnvSelected(g.Environment) {
				continue
			}
			if len(g.Files) == 0 {
				if len(g.Templates) > 0 {
					m.TemplateOnly = append(m.TemplateOnly, g.Environment)
				}
				continue
			}
			env := g.Environment
			m.Environments = append(m.Environments, env)
			defined[env] = map[string]CellState{}
			declared[env] = map[string]bool{}
			for _, f := range g.Files {
				m.Files[env] = append(m.Files[env], f.Path)
				for _, e := range f.Entries() {
					st := CellSet
					if e.Value == "" {
						st = CellEmpty
					}
					defined[env][e.Key] = st // the last definition wins
					if !p.ProcessEnv {
						rows[e.Key] = true
					}
				}
			}
			for _, t := range a.ApplicableTemplates(env, false) {
				for _, e := range t.Entries() {
					if rules.IsRequired(e) {
						declared[env][e.Key] = true
					}
					rows[e.Key] = true
				}
			}
		}
		if len(m.Environments) == 0 && len(m.TemplateOnly) == 0 {
			continue
		}
		names := make([]string, 0, len(rows))
		for k := range rows {
			names = append(names, k)
		}
		slices.Sort(names)
		for _, k := range names {
			row := MatrixRow{Name: k, Cells: map[string]CellState{}}
			for _, env := range m.Environments {
				st, ok := defined[env][k]
				switch {
				case ok:
				case declared[env][k]:
					st = CellMissing
				default:
					st = CellAbsent
				}
				row.Cells[env] = st
			}
			m.Variables = append(m.Variables, row)
		}
		out = append(out, m)
	}
	return out
}

// EnvDiff compares the variable names of two environments per app root.
type EnvDiff struct {
	A    string    `json:"a"`
	B    string    `json:"b"`
	Apps []AppDiff `json:"apps"`
}

// AppDiff is the difference in one app root. A side without a real env file
// is represented by its templates (FromTemplates).
type AppDiff struct {
	Root           string   `json:"app_root"`
	AFiles         []string `json:"a_files"`
	BFiles         []string `json:"b_files"`
	AFromTemplates bool     `json:"a_from_templates"`
	BFromTemplates bool     `json:"b_from_templates"`
	OnlyInA        []string `json:"only_in_a"`
	OnlyInB        []string `json:"only_in_b"`
	Common         int      `json:"common"`
}

// Differences counts the variables that differ.
func (d *EnvDiff) Differences() int {
	n := 0
	for _, a := range d.Apps {
		n += len(a.OnlyInA) + len(a.OnlyInB)
	}
	return n
}

// ErrUnknownEnvironment is returned by BuildDiff for an environment that has
// neither env files nor templates.
var ErrUnknownEnvironment = errors.New("unknown environment")

// Environments lists every environment with env files or templates.
func Environments(p *project.Project) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range p.Apps {
		for _, g := range a.Groups {
			if !seen[g.Environment] {
				seen[g.Environment] = true
				out = append(out, g.Environment)
			}
		}
	}
	slices.Sort(out)
	return out
}

// BuildDiff compares environments a and b (aliases allowed). The error
// never echoes the arguments.
func BuildDiff(p *project.Project, app, a, b string) (*EnvDiff, error) {
	a, b = discovery.NormalizeEnvironment(a), discovery.NormalizeEnvironment(b)
	known := Environments(p)
	for i, env := range []string{a, b} {
		if !slices.Contains(known, env) {
			which := [...]string{"first", "second"}[i]
			return nil, fmt.Errorf("%w (%s argument): no env file or template for it; known environments: %s",
				ErrUnknownEnvironment, which, mask.Printable(strings.Join(known, ", ")))
		}
	}
	d := &EnvDiff{A: a, B: b, Apps: []AppDiff{}}
	for _, ap := range p.Apps {
		if app != "" && app != "." && ap.Root != app {
			continue
		}
		ka, fa, ta := envKeys(ap, a)
		kb, fb, tb := envKeys(ap, b)
		if len(fa) == 0 && len(fb) == 0 {
			continue
		}
		ad := AppDiff{Root: ap.Root, AFiles: fa, BFiles: fb, AFromTemplates: ta, BFromTemplates: tb,
			OnlyInA: []string{}, OnlyInB: []string{}}
		for k := range ka {
			if kb[k] {
				ad.Common++
			} else {
				ad.OnlyInA = append(ad.OnlyInA, k)
			}
		}
		for k := range kb {
			if !ka[k] {
				ad.OnlyInB = append(ad.OnlyInB, k)
			}
		}
		slices.Sort(ad.OnlyInA)
		slices.Sort(ad.OnlyInB)
		d.Apps = append(d.Apps, ad)
	}
	return d, nil
}

// envKeys returns the variable names of an environment in an app: from its
// real env files, or, if it has none, from the templates that apply to it.
func envKeys(a *project.App, env string) (keys map[string]bool, files []string, fromTemplates bool) {
	keys = map[string]bool{}
	files = []string{}
	g := a.Group(env)
	if g != nil && len(g.Files) > 0 {
		for _, f := range g.Files {
			files = append(files, f.Path)
			for _, k := range f.Keys() {
				keys[k] = true
			}
		}
		return keys, files, false
	}
	if g == nil {
		return keys, files, false
	}
	for _, t := range a.ApplicableTemplates(env, false) {
		files = append(files, t.Path)
		for _, k := range t.Keys() {
			keys[k] = true
		}
	}
	return keys, files, true
}

// MatrixText renders the environment matrices for the terminal.
func MatrixText(w io.Writer, ms []AppMatrix, opts Options) error {
	ms = printableMatrices(ms)
	s := newStyler(opts)
	var b strings.Builder
	if len(ms) == 0 {
		b.WriteString("No env files or templates found.\n")
	}
	for i := range ms {
		if i > 0 {
			b.WriteByte('\n')
		}
		writeMatrix(&b, s, &ms[i], "")
	}
	if len(ms) > 0 {
		b.WriteByte('\n')
		b.WriteString(legend(s))
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, opts.finish(trimRight(b.String())))
	return err
}

func legend(s *styler) string {
	return s.paint(s.dim, fmt.Sprintf("%s set  %s empty  %s missing (required by a template)  %s not required",
		s.sym.Set, s.sym.Empty, s.sym.Missing, s.sym.None))
}

func (s *styler) cell(st CellState) string {
	switch st {
	case CellSet:
		return s.paint(s.green, s.sym.Set)
	case CellEmpty:
		return s.paint(s.yellow, s.sym.Empty)
	case CellMissing:
		return s.paint(s.red, s.sym.Missing)
	default:
		return s.paint(s.dim, s.sym.None)
	}
}

func cellVisible(sym symbols, st CellState) string {
	switch st {
	case CellSet:
		return sym.Set
	case CellEmpty:
		return sym.Empty
	case CellMissing:
		return sym.Missing
	default:
		return sym.None
	}
}

func writeMatrix(b *strings.Builder, s *styler, m *AppMatrix, indent string) {
	title := "App " + m.Root
	if m.Root == "." {
		title = "App . (repository root)"
	}
	b.WriteString(indent + s.paint(s.bold, title) + "\n")
	if len(m.Environments) == 0 {
		b.WriteString(indent + "  No real env files yet.\n")
	}
	if len(m.Environments) > 0 && len(m.Variables) == 0 {
		b.WriteString(indent + "  No variables.\n")
	}
	if len(m.Environments) > 0 && len(m.Variables) > 0 {
		nameW := len("VARIABLE")
		for _, r := range m.Variables {
			nameW = max(nameW, min(utf8.RuneCountInString(r.Name), maxNameWidth))
		}
		heads := make([]string, len(m.Environments))
		b.WriteString(indent + "  " + padRight("VARIABLE", s.paint(s.dim, "VARIABLE"), nameW))
		for i, env := range m.Environments {
			heads[i] = mask.Clip(env, maxEnvWidth)
			b.WriteString("  " + s.paint(s.dim, heads[i]))
		}
		b.WriteByte('\n')
		for _, r := range m.Variables {
			name := mask.Clip(r.Name, maxNameWidth)
			b.WriteString(indent + "  " + padRight(name, name, nameW))
			for i, env := range m.Environments {
				vis := cellVisible(s.sym, r.Cells[env])
				colW := max(utf8.RuneCountInString(heads[i]), 1)
				left := (colW - utf8.RuneCountInString(vis)) / 2
				b.WriteString("  " + strings.Repeat(" ", left) + s.cell(r.Cells[env]) +
					strings.Repeat(" ", max(0, colW-left-utf8.RuneCountInString(vis))))
			}
			b.WriteString("\n")
		}
	}
	if len(m.TemplateOnly) > 0 {
		b.WriteString(indent + "  " + s.paint(s.dim, "Templates only, no env file yet: "+strings.Join(m.TemplateOnly, ", ")) + "\n")
	}
}

// trimRight removes trailing blanks on every line (centered cells pad).
func trimRight(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// DiffText renders an environment diff for the terminal.
func DiffText(w io.Writer, d *EnvDiff, opts Options) error {
	d = printableDiff(d)
	s := newStyler(opts)
	var b strings.Builder
	b.WriteString(s.paint(s.bold, fmt.Sprintf("Comparing %s %s %s", d.A, s.sym.Arrow, d.B)) + "\n")
	differing := 0
	for _, a := range d.Apps {
		b.WriteByte('\n')
		src := func(files []string, tmpl bool) string {
			if len(files) == 0 {
				return "nothing"
			}
			t := strings.Join(files, ", ")
			if tmpl {
				t += " (templates)"
			}
			return t
		}
		b.WriteString(s.paint(s.bold, "App "+a.Root) + "\n")
		b.WriteString(s.paint(s.dim, fmt.Sprintf("  %s: %s\n  %s: %s", d.A, src(a.AFiles, a.AFromTemplates), d.B,
			src(a.BFiles, a.BFromTemplates))) + "\n")
		if len(a.OnlyInA)+len(a.OnlyInB) == 0 {
			b.WriteString("  " + s.paint(s.green, s.sym.OK) + fmt.Sprintf(" same %s\n", plural(a.Common, "variable", "variables")))
			continue
		}
		differing++
		nameW := 0
		for _, k := range append(slices.Clone(a.OnlyInA), a.OnlyInB...) {
			nameW = max(nameW, min(utf8.RuneCountInString(k), maxNameWidth))
		}
		minus, plus := "-", "+"
		for _, k := range a.OnlyInA {
			k = mask.Clip(k, maxNameWidth)
			b.WriteString("  " + s.paint(s.red, minus) + " " + padRight(k, s.paint(s.bold, k), nameW) + "  " +
				s.paint(s.dim, "only in "+d.A) + "\n")
		}
		for _, k := range a.OnlyInB {
			k = mask.Clip(k, maxNameWidth)
			b.WriteString("  " + s.paint(s.green, plus) + " " + padRight(k, s.paint(s.bold, k), nameW) + "  " +
				s.paint(s.dim, "only in "+d.B) + "\n")
		}
		if a.Common > 0 {
			b.WriteString(s.paint(s.dim, fmt.Sprintf("  %s in both", plural(a.Common, "variable", "variables"))) + "\n")
		}
	}
	b.WriteByte('\n')
	n := d.Differences()
	if n == 0 {
		b.WriteString(s.paint(s.green, s.sym.OK+" No differences") + fmt.Sprintf(": %s and %s define the same variables.\n", d.A, d.B))
	} else {
		fmt.Fprintf(&b, "%s differ in %s.\n", plural(n, "variable", "variables"), plural(differing, "app root", "app roots"))
	}
	_, err := io.WriteString(w, opts.finish(trimRight(b.String())))
	return err
}

// DiffJSON writes the diff of two environments as JSON; see
// docs/json-output.md.
func DiffJSON(w io.Writer, version, root string, d *EnvDiff) error {
	return writeJSON(w, struct {
		SchemaVersion int      `json:"schema_version"`
		Tool          Tool     `json:"tool"`
		Command       string   `json:"command"`
		Root          string   `json:"root"`
		Diff          *EnvDiff `json:"diff"`
	}{SchemaVersion, Tool{Name: "envguard", Version: version}, "diff", root, d})
}

// MatrixJSON writes the environment matrix as JSON; see docs/json-output.md.
func MatrixJSON(w io.Writer, version, root string, ms []AppMatrix) error {
	return writeJSON(w, struct {
		SchemaVersion int         `json:"schema_version"`
		Tool          Tool        `json:"tool"`
		Command       string      `json:"command"`
		Root          string      `json:"root"`
		Matrix        []AppMatrix `json:"matrix"`
	}{SchemaVersion, Tool{Name: "envguard", Version: version}, "diff", root, ms})
}

func writeJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}
