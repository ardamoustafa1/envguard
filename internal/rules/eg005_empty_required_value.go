package rules

import (
	"fmt"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/parser"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(emptyRequiredValue{base{"EG005", "empty-required-value", findings.Warning}}) }

// emptyRequiredValue reports variables that are present but empty (or only
// whitespace) in a real env file although an applicable template requires
// them. Template convention: a declared variable is required if the template
// gives it a non-empty value, or if its line is not marked `# optional`.
// For duplicated keys the last definition (the effective one) is checked.
type emptyRequiredValue struct{ base }

func (emptyRequiredValue) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	for _, app := range p.Apps {
		for _, g := range app.Groups {
			if !p.EnvSelected(g.Environment) {
				continue
			}
			for _, f := range g.Files {
				decls, _ := declarations(app.ApplicableTemplates(g.Environment, f.Local))
				required := map[string]*project.SchemaVar{}
				for _, sv := range schemaRequired(p, app, g.Environment) {
					required[sv.Name] = sv
				}
				for _, e := range lastEntries(f) {
					if strings.TrimSpace(e.Value) != "" {
						continue
					}
					d, ok := decls[e.Key]
					if !ok || !IsRequired(d.Entry) {
						if sv := required[e.Key]; sv != nil {
							out = append(out, emptySchemaValue(p, f, e.Key, e.KeyPos.Line, e.KeyPos.Column, sv))
						}
						continue
					}
					fix := fmt.Sprintf("Set a value for %s in %s:%d.", e.Key, f.Path, e.KeyPos.Line)
					if d.Entry.Value == "" {
						fix += fmt.Sprintf(" If an empty value is intentional, mark the template line optional: "+
							"`%s= # optional` in %s.", e.Key, d.ref())
					} else {
						fix += fmt.Sprintf(" The template suggests a non-empty value (%s), so an empty one is "+
							"almost certainly a mistake; if it is intentional, add `# envguard-ignore EG005` "+
							"on the line above it.", d.ref())
					}
					out = append(out, findings.Finding{
						Message:  fmt.Sprintf("%s is empty in %s but required by %s", e.Key, where(f), d.File.Path),
						Variable: e.Key,
						File:     f.Path,
						Line:     e.KeyPos.Line,
						Column:   e.KeyPos.Column,
						Why: fmt.Sprintf("An empty %s is usually read as \"set\" and passes presence checks, "+
							"then fails later (empty URLs, empty credentials, zero ports).", e.Key),
						Fix:     fix,
						Related: []findings.Location{d.location()},
					})
				}
			}
		}
	}
	return out
}

func emptySchemaValue(p *project.Project, f *project.EnvFile, key string, line, col int, sv *project.SchemaVar) findings.Finding {
	loc := p.Schema.Location(key)
	at := fmt.Sprintf("%s:%d", f.Path, line)
	if f.Path == project.ProcessEnvPath {
		at = "the process environment"
	}
	return findings.Finding{
		Message:  fmt.Sprintf("%s is empty in %s but required by the schema (%s:%d)", key, where(f), loc.File, loc.Line),
		Variable: key,
		File:     f.Path,
		Line:     line,
		Column:   col,
		Why: fmt.Sprintf("An empty %s is usually read as \"set\" and passes presence checks, then fails later; "+
			"the schema requires a real value.", key),
		Fix:     fmt.Sprintf("Set %s to %s in %s.", key, sv.Spec.Describe(), at),
		Related: []findings.Location{{File: loc.File, Line: loc.Line, Column: loc.Column}},
	}
}

// lastEntries returns the last definition of each key, in file order of
// those definitions.
func lastEntries(f *project.EnvFile) []parser.Entry {
	entries := f.Entries()
	last := map[string]int{}
	for i, e := range entries {
		last[e.Key] = i
	}
	var out []parser.Entry
	for i, e := range entries {
		if last[e.Key] == i {
			out = append(out, e)
		}
	}
	return out
}
