package report

import (
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/mask"
)

// Names in a report (file paths, variable names, environment names) come
// from the analyzed repository and may contain terminal escape sequences or
// other characters that would fake or hide output. The text renderers
// (pretty, doctor, github, matrix and diff) therefore work on a copy in
// which mask.Printable has replaced them with visible escapes. JSON and
// SARIF keep the raw strings: their encoders escape them.

const (
	// maxNameWidth caps the width of aligned name columns, so one huge name
	// cannot pad every row of a table to its length.
	maxNameWidth = 48
	maxEnvWidth  = 24
)

func (r *Report) printable() *Report {
	c := *r
	c.Root, c.App, c.GitReason = mask.Printable(r.Root), mask.Printable(r.App), mask.Printable(r.GitReason)
	c.PathPrefix = mask.Printable(r.PathPrefix)
	c.Environments = printableStrings(r.Environments)
	c.Findings = make([]findings.Finding, len(r.Findings))
	for i := range r.Findings {
		f := r.Findings[i]
		f.Message, f.Variable, f.File = mask.Printable(f.Message), mask.Printable(f.Variable), mask.Printable(f.File)
		f.Evidence, f.Why, f.Fix = mask.Printable(f.Evidence), mask.Printable(f.Why), mask.Printable(f.Fix)
		f.Related = printableLocations(f.Related)
		c.Findings[i] = f
	}
	c.Matrix = printableMatrices(r.Matrix)
	c.CI = CIInventory{Secrets: printableCI(r.CI.Secrets), Vars: printableCI(r.CI.Vars)}
	c.Notes = make([]Note, len(r.Notes))
	for i, n := range r.Notes {
		n.File, n.Variable, n.Text = mask.Printable(n.File), mask.Printable(n.Variable), mask.Printable(n.Text)
		c.Notes[i] = n
	}
	c.Stats.AppRoots = printableStrings(r.Stats.AppRoots)
	c.Stats.Languages = make([]LanguageCount, len(r.Stats.Languages))
	for i, l := range r.Stats.Languages {
		c.Stats.Languages[i] = LanguageCount{Language: mask.Printable(l.Language), Files: l.Files}
	}
	return &c
}

func printableStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = mask.Printable(s)
	}
	return out
}

func printableLocations(in []findings.Location) []findings.Location {
	if in == nil {
		return nil
	}
	out := make([]findings.Location, len(in))
	for i, l := range in {
		l.File = mask.Printable(l.File)
		out[i] = l
	}
	return out
}

func printableCI(in []CIItem) []CIItem {
	out := make([]CIItem, len(in))
	for i, it := range in {
		out[i] = CIItem{Name: mask.Printable(it.Name), Refs: printableLocations(it.Refs)}
	}
	return out
}

func printableMatrices(in []AppMatrix) []AppMatrix {
	if in == nil {
		return nil
	}
	out := make([]AppMatrix, len(in))
	for i, m := range in {
		c := AppMatrix{Root: mask.Printable(m.Root), Environments: printableStrings(m.Environments),
			TemplateOnly: printableStrings(m.TemplateOnly), Files: map[string][]string{},
			Variables: make([]MatrixRow, len(m.Variables))}
		for env, files := range m.Files {
			c.Files[mask.Printable(env)] = printableStrings(files)
		}
		for j, row := range m.Variables {
			cells := make(map[string]CellState, len(row.Cells))
			for env, st := range row.Cells {
				cells[mask.Printable(env)] = st
			}
			c.Variables[j] = MatrixRow{Name: mask.Printable(row.Name), Cells: cells}
		}
		out[i] = c
	}
	return out
}

func printableDiff(d *EnvDiff) *EnvDiff {
	c := &EnvDiff{A: mask.Printable(d.A), B: mask.Printable(d.B), Apps: make([]AppDiff, len(d.Apps))}
	for i, a := range d.Apps {
		a.Root = mask.Printable(a.Root)
		a.AFiles, a.BFiles = printableStrings(a.AFiles), printableStrings(a.BFiles)
		a.OnlyInA, a.OnlyInB = printableStrings(a.OnlyInA), printableStrings(a.OnlyInB)
		c.Apps[i] = a
	}
	return c
}
