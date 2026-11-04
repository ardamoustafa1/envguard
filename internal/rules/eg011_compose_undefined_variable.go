package rules

import (
	"fmt"
	"path"

	"github.com/ardamoustafa1/envguard/internal/compose"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/scanner"
)

func init() {
	Register(composeUndefinedVariable{base{"EG011", "compose-undefined-variable", findings.Warning}})
}

// composeUndefinedVariable reports two Compose problems:
//
//  1. ${VAR} interpolated without a default although it is not defined in
//     the .env next to the Compose file (the file Compose reads), nor in a
//     template (in that directory or of the file's app root and its
//     ancestors), nor in the schema. If that .env does not exist, the message
//     says so and only templates and schema count. One finding per file and
//     variable, at the first occurrence; ${VAR:?err} is included (Compose
//     aborts), well-known host variables and ignore_variables are not.
//  2. env_file: entries that do not exist, unless `required: false`.
type composeUndefinedVariable struct{ base }

func (composeUndefinedVariable) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	ignore := ignoreVariables(p)
	for _, cf := range p.ComposeFiles {
		if cf.Problem != nil {
			continue
		}
		adjacent := p.File(cf.AdjacentEnv())
		defined, templates := composeDefinitions(p, cf, adjacent)

		seen := map[string]bool{}
		for i, it := range cf.Interpolations {
			if it.HasDefault || defined[it.Variable] || seen[it.Variable] ||
				scanner.IsWellKnown(it.Variable, false) || matchesAny(ignore, it.Variable) {
				continue
			}
			seen[it.Variable] = true
			var related []findings.Location
			for _, other := range cf.Interpolations[i+1:] {
				if other.Variable == it.Variable && len(related) < maxRelatedUsages {
					related = append(related, findings.Location{File: cf.Path, Line: other.Line, Column: other.Column})
				}
			}
			out = append(out, undefinedInterpolation(cf, it, adjacent, templates, related))
		}

		for _, svc := range cf.Services {
			for _, ef := range svc.EnvFiles {
				if ef.Exists || !ef.Required {
					continue
				}
				out = append(out, missingEnvFile(cf, svc, ef))
			}
		}
	}
	return out
}

// composeDefinitions returns the variables available to interpolation and
// the template paths considered.
func composeDefinitions(p *project.Project, cf *compose.File, adjacent *project.EnvFile) (defined map[string]bool, templatePaths []string) {
	defined = map[string]bool{}
	if adjacent != nil {
		for _, k := range adjacent.Keys() {
			defined[k] = true
		}
	}
	var templates []string
	addTemplates := func(fs []*project.EnvFile) {
		for _, t := range fs {
			templates = append(templates, t.Path)
			for _, k := range t.Keys() {
				defined[k] = true
			}
		}
	}
	for _, f := range p.EnvFiles {
		if f.IsTemplate() && path.Dir(f.Path) == cf.Dir {
			addTemplates([]*project.EnvFile{f})
		}
	}
	if df := p.Inventory; df != nil {
		for _, f := range df.Files {
			if f.Path == cf.Path {
				apps := append([]*project.App{p.AppFor(f.AppRoot)}, p.AncestorApps(f.AppRoot)...)
				for _, a := range apps {
					if a != nil {
						addTemplates(a.Templates())
					}
				}
			}
		}
	}
	if p.Settings != nil {
		for k := range p.Settings.Variables {
			defined[k] = true
		}
	}
	return defined, uniqueStrings(templates)
}

func undefinedInterpolation(cf *compose.File, it compose.Interpolation, adjacent *project.EnvFile,
	templates []string, related []findings.Location) findings.Finding {
	envPath := cf.AdjacentEnv()
	var msg, fix string
	if adjacent != nil {
		msg = fmt.Sprintf("%s interpolates ${%s}, which is not defined in %s (the file Compose reads) or any template",
			cf.Path, it.Variable, envPath)
		fix = fmt.Sprintf("Add `%s=` with a value to %s and document it in a template, or give it a default in "+
			"the Compose file: `${%s:-value}`.", it.Variable, envPath, it.Variable)
	} else {
		checked := "no template declares it"
		if len(templates) > 0 {
			checked = "it is not declared in " + joinList(templates)
		}
		msg = fmt.Sprintf("%s interpolates ${%s}, but there is no %s next to it and %s", cf.Path, it.Variable, envPath, checked)
		fix = fmt.Sprintf("Create %s with `%s=<value>` (Compose reads it automatically), declare `%s=` in a template "+
			"so others know to set it, or give it a default: `${%s:-value}`.", envPath, it.Variable, it.Variable, it.Variable)
	}
	why := fmt.Sprintf("Compose substitutes an empty string for an unset %s and only prints a warning, so the "+
		"container starts with a broken value.", it.Variable)
	if it.Required {
		why = fmt.Sprintf("The reference uses ${%s:?...}, so `docker compose up` aborts when it is unset.", it.Variable)
	}
	return findings.Finding{
		Message:  msg,
		Variable: it.Variable,
		File:     cf.Path,
		Line:     it.Line,
		Column:   it.Column,
		Why:      why,
		Fix:      fix,
		Related:  related,
	}
}

func missingEnvFile(cf *compose.File, svc compose.Service, ef compose.EnvFile) findings.Finding {
	return findings.Finding{
		Message: fmt.Sprintf("service %s: env_file %s does not exist", svc.Name, ef.Path),
		File:    cf.Path,
		Line:    ef.Line,
		Column:  ef.Column,
		Why:     "`docker compose up` fails for this service when a required env_file is missing.",
		Fix: fmt.Sprintf("Create %s (for example by copying its template), or make it optional with the long syntax:\n\n"+
			"    env_file:\n      - path: %s\n        required: false", ef.Path, ef.Written),
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
