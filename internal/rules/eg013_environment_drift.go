package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(environmentDrift{base{"EG013", "environment-drift", findings.Warning}}) }

// environmentDrift compares the variable names of environment-specific
// templates (.env.staging.example vs .env.production.example) within one
// app root. Variables in the default template apply to every environment
// and are ignored, as are local templates. At least two environments with
// templates are needed. One finding per variable, at its first declaration.
type environmentDrift struct{ base }

func (environmentDrift) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	for _, app := range p.Apps {
		shared := map[string]bool{}
		if g := app.Group(discovery.EnvDefault); g != nil {
			for _, t := range g.Templates {
				if !t.Local {
					for _, k := range t.Keys() {
						shared[k] = true
					}
				}
			}
		}

		type envTemplates struct {
			env   string
			files []*project.EnvFile
			keys  map[string]decl
		}
		var envs []envTemplates
		var order []string
		seen := map[string]bool{}
		for _, g := range app.Groups {
			if g.Environment == discovery.EnvDefault {
				continue
			}
			var files []*project.EnvFile
			for _, t := range g.Templates {
				if !t.Local {
					files = append(files, t)
				}
			}
			if len(files) == 0 {
				continue
			}
			keys, keyOrder := declarations(files)
			envs = append(envs, envTemplates{g.Environment, files, keys})
			for _, k := range keyOrder {
				if !seen[k] {
					seen[k] = true
					order = append(order, k)
				}
			}
		}
		if len(envs) < 2 {
			continue
		}

		for _, key := range order {
			if shared[key] {
				continue
			}
			var has, missing, missingFiles []string
			var first *decl
			var related []findings.Location
			selected := false
			for _, e := range envs {
				selected = selected || p.EnvSelected(e.env)
				if d, ok := e.keys[key]; ok {
					has = append(has, e.env)
					if first == nil {
						first = &d
					} else {
						related = append(related, d.location())
					}
				} else {
					missing = append(missing, e.env)
					missingFiles = append(missingFiles, e.files[0].Path)
				}
			}
			if len(missing) == 0 || !selected {
				continue
			}
			out = append(out, findings.Finding{
				Message: fmt.Sprintf("%s is declared for %s but not for %s",
					key, joinList(has), joinList(missing)),
				Variable: key,
				File:     first.File.Path,
				Line:     first.Entry.KeyPos.Line,
				Column:   first.Entry.KeyPos.Column,
				Why: "Environments that are supposed to run the same code should need the same variables; " +
					"a variable documented for only some of them is often forgotten when the next deploy reaches the others.",
				Fix: fmt.Sprintf("Add `%s=` to %s, or move it to %s if every environment needs it. "+
					"If it really applies only to %s, add `# envguard-ignore EG013` above %s.",
					key, joinList(missingFiles), templateFor(app, discovery.EnvDefault), joinList(has), first.ref()),
				Related: related,
			})
		}
	}
	return out
}
