package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(missingVariable{base{"EG001", "missing-variable", findings.Error}}) }

// missingVariable compares each real env file with what its environment
// requires:
//
//   - the templates that apply to it: the default templates (.env.example)
//     plus the environment's own (.env.production.example). Variables marked
//     optional (`KEY= # optional`) are not required.
//   - schema variables with `required: true` whose `environments` include the
//     file's environment (all environments if unset), for app roots that
//     reference the variable (in a template or in code) and for the root app.
//
// Local override files (.env.local, .env.production.local) are skipped, and
// missing env files themselves are not reported. In --process-env mode the
// process environment is the "file".
type missingVariable struct{ base }

func (missingVariable) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	for _, app := range p.Apps {
		for _, g := range app.Groups {
			if !p.EnvSelected(g.Environment) {
				continue
			}
			decls, order := declarations(app.ApplicableTemplates(g.Environment, false))
			for _, f := range g.Files {
				if f.Local || f.Parsed == nil {
					continue
				}
				have := map[string]bool{}
				for _, k := range f.Keys() {
					have[k] = true
				}
				for _, key := range order {
					d := decls[key]
					if have[key] || !IsRequired(d.Entry) {
						continue
					}
					out = append(out, missingFromTemplate(f, g.Environment, key, d))
				}
				for _, sv := range schemaRequired(p, app, g.Environment) {
					if have[sv.Name] {
						continue
					}
					if d, ok := decls[sv.Name]; ok && IsRequired(d.Entry) {
						continue // already reported via the template
					}
					out = append(out, missingFromSchema(p, f, g.Environment, sv))
				}
			}
		}
	}
	return out
}

func missingFromTemplate(f *project.EnvFile, env, key string, d decl) findings.Finding {
	fi := findings.Finding{
		Message:  fmt.Sprintf("%s is declared in %s but missing from %s", key, d.File.Path, f.Path),
		Variable: key,
		File:     f.Path,
		Why: fmt.Sprintf("Anything that reads %s in the %s environment gets no value, which usually fails "+
			"only at runtime or silently falls back to a default.", key, env),
		Fix: fmt.Sprintf("Add `%s=` to %s (it is declared in %s) and set its value. "+
			"If it is not needed there, mark it optional in the template (`%s= # optional`) "+
			"or put `# envguard-ignore EG001` on the line above it in %s.",
			key, f.Path, d.ref(), key, d.File.Path),
		Related: []findings.Location{d.location()},
	}
	if f.Path == project.ProcessEnvPath {
		fi.Message = fmt.Sprintf("%s is declared in %s but not set in the process environment", key, d.File.Path)
		fi.Fix = fmt.Sprintf("Set %s in the container, CI job or platform settings for the %s environment "+
			"(it is declared in %s), or mark it optional in the template (`%s= # optional`).", key, env, d.ref(), key)
	}
	return fi
}

func missingFromSchema(p *project.Project, f *project.EnvFile, env string, sv *project.SchemaVar) findings.Finding {
	loc := p.Schema.Location(sv.Name)
	ref := fmt.Sprintf("%s:%d", loc.File, loc.Line)
	scope := "every environment"
	if len(sv.Config.Environments) > 0 {
		scope = joinList(sv.Config.Environments)
	}
	fi := findings.Finding{
		Message:  fmt.Sprintf("%s is required by the schema (%s) but missing from %s", sv.Name, ref, f.Path),
		Variable: sv.Name,
		File:     f.Path,
		Why: fmt.Sprintf("The schema marks %s as required in %s; without it the app starts misconfigured in the %s "+
			"environment.", sv.Name, scope, env),
		Fix: fmt.Sprintf("Add `%s=` with a value (%s) to %s. If it is not needed in %s, narrow "+
			"`environments:` for %s in %s.", sv.Name, sv.Spec.Describe(), f.Path, env, sv.Name, ref),
		Related: []findings.Location{{File: loc.File, Line: loc.Line, Column: loc.Column}},
	}
	if f.Path == project.ProcessEnvPath {
		fi.Message = fmt.Sprintf("%s is required by the schema (%s) but not set in the process environment", sv.Name, ref)
		fi.Fix = fmt.Sprintf("Set %s (%s) in the container, CI job or platform settings for the %s environment, "+
			"or narrow `environments:` for it in %s.", sv.Name, sv.Spec.Describe(), env, ref)
	}
	return fi
}

// schemaRequired returns the schema variables required in env that apply
// to app: in the root app, in --process-env mode (the user picked the app),
// or when the app's templates or code reference the variable. Sorted by name.
func schemaRequired(p *project.Project, app *project.App, env string) []*project.SchemaVar {
	if p.Schema == nil {
		return nil
	}
	var refs map[string]bool
	var out []*project.SchemaVar
	for _, name := range p.Schema.Names() {
		sv := p.Schema.Variables[name]
		if !sv.Config.RequiredIn(env) {
			continue
		}
		if app.Root != "." && !p.ProcessEnv {
			if refs == nil {
				refs = appReferences(p, app)
			}
			if !refs[name] {
				continue
			}
		}
		out = append(out, sv)
	}
	return out
}

// appReferences lists variables an app's templates or code mention.
func appReferences(p *project.Project, app *project.App) map[string]bool {
	refs := map[string]bool{}
	for _, t := range app.Templates() {
		for _, k := range t.Keys() {
			refs[k] = true
		}
	}
	for _, u := range p.UsagesUnder(app.Root) {
		if u.Certain() {
			refs[u.Variable] = true
		}
	}
	return refs
}

// schemaDocumented reports whether the schema declares name.
func schemaDocumented(p *project.Project, name string) bool {
	return p.SchemaVar(name) != nil
}
