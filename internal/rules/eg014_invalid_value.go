package rules

import (
	"fmt"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(invalidValue{base{"EG014", "invalid-value", findings.Error}}) }

// invalidValue validates the values of real env files (or the process
// environment in --process-env mode) against the schema's type and
// constraints. Only the effective (last) definition of a key is checked,
// empty values are left to EG005, and templates are not checked (they hold
// placeholders). The message names what was expected; the actual value is
// shown only through Project.DisplayValue, and never for sensitive variables.
type invalidValue struct{ base }

func (invalidValue) Check(p *project.Project) []findings.Finding {
	if p.Schema == nil {
		return nil
	}
	var out []findings.Finding
	for _, f := range p.EnvFiles {
		if f.IsTemplate() || !p.EnvSelected(f.Environment) {
			continue
		}
		for _, e := range lastEntries(f) {
			sv := p.SchemaVar(e.Key)
			if sv == nil || strings.TrimSpace(e.Value) == "" {
				continue
			}
			reason, ok := sv.Spec.Check(e.Value)
			if ok {
				continue
			}
			loc := p.Schema.Location(e.Key)
			msg := fmt.Sprintf("%s in %s has an invalid value: %s", e.Key, where(f), reason)
			if !p.Sensitive(e.Key) {
				msg = fmt.Sprintf("%s in %s has the invalid value %s: %s", e.Key, where(f), p.DisplayValue(e.Key, e.Value), reason)
			}
			at := fmt.Sprintf("%s:%d", f.Path, e.KeyPos.Line)
			if f.Path == project.ProcessEnvPath {
				at = "the process environment"
			}
			fix := fmt.Sprintf("Set %s to %s in %s.", e.Key, sv.Spec.Describe(), at)
			if sv.Config.Default != "" && !p.Sensitive(e.Key) {
				fix += fmt.Sprintf(" The schema's default is %s.", p.DisplayValue(e.Key, sv.Config.Default))
			}
			fix += fmt.Sprintf(" If the schema is wrong, update %s at %s:%d.", e.Key, loc.File, loc.Line)
			out = append(out, findings.Finding{
				Message:  msg,
				Variable: e.Key,
				File:     f.Path,
				Line:     e.ValuePos.Line,
				Column:   e.ValuePos.Column,
				Why: fmt.Sprintf("The app expects %s to be %s; an invalid value usually crashes it at startup or, "+
					"worse, is silently misread.", e.Key, sv.Spec.Describe()),
				Fix:     fix,
				Related: []findings.Location{{File: loc.File, Line: loc.Line, Column: loc.Column}},
			})
		}
	}
	return out
}
