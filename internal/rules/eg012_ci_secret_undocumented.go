package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(ciSecretUndocumented{base{"EG012", "ci-secret-undocumented", findings.Info}}) }

// ciSecretUndocumented reports secrets and configuration variables that
// workflows reference (project.CI) but that no template anywhere in the
// repository and no schema variable documents. GITHUB_TOKEN is never
// reported. One finding per name, at its first reference.
type ciSecretUndocumented struct{ base }

func (ciSecretUndocumented) Check(p *project.Project) []findings.Finding {
	documented := map[string]bool{}
	for _, f := range p.EnvFiles {
		if f.IsTemplate() {
			for _, k := range f.Keys() {
				documented[k] = true
			}
		}
	}
	if p.Settings != nil {
		for k := range p.Settings.Variables {
			documented[k] = true
		}
	}
	ignore := ignoreVariables(p)
	target := suggestTemplate(p, ".")

	var out []findings.Finding
	report := func(items []project.CIItem, what, settingsPage string) {
		for _, item := range items {
			if documented[item.Name] || matchesAny(ignore, item.Name) || len(item.Refs) == 0 {
				continue
			}
			first := item.Refs[0]
			var related []findings.Location
			for _, r := range item.Refs[1:min(len(item.Refs), 1+maxRelatedUsages)] {
				related = append(related, findings.Location{File: r.File, Line: r.Line, Column: r.Column})
			}
			out = append(out, findings.Finding{
				Message:  fmt.Sprintf("workflows use %s %s, but no template or schema documents it", what, item.Name),
				Variable: item.Name,
				File:     first.File,
				Line:     first.Line,
				Column:   first.Column,
				Why: fmt.Sprintf("Whoever sets up a fork, a new environment or a new repository has to discover %s by "+
					"reading workflow files; a missing one usually shows up as a failed deploy.", item.Name),
				Fix: fmt.Sprintf("Document it: add `%s=` to %s with a comment such as `# CI: set in GitHub → Settings → "+
					"Secrets and variables → Actions (%s)`, or declare it under `variables:` in .envguard.yaml.",
					item.Name, target, settingsPage),
				Related: related,
			})
		}
	}
	report(p.CI.Secrets, "secret", "Secrets tab")
	report(p.CI.Vars, "variable", "Variables tab")
	return out
}
