package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/scanner"
)

func init() { Register(usedButUndeclared{base{"EG003", "used-but-undeclared", findings.Error}}) }

// usedButUndeclared reports variables the code reads that nothing declares:
// no env file or template anywhere in the repository, no schema variable,
// no Compose definition and no workflow env entry. Declarations are matched
// repository-wide (monorepos often share one template) for precision.
// One finding per app root and variable, at the first usage, with up to 3
// more usages in Related. Error when the code needs the value; info when
// every usage has a default (the app works without it) and also when the
// repository declares nothing at all (no env file, template, Compose or
// workflow definition, schema): a library or CLI that reads a few optional
// variables has nothing to disagree with. Dynamic lookups, inferred names,
// well-known platform variables (scanner.IsWellKnown) and
// scanner.ignore_variables are never reported.
type usedButUndeclared struct{ base }

const maxRelatedUsages = 3

func (usedButUndeclared) Check(p *project.Project) []findings.Finding {
	declared := declaredEverywhere(p)
	declaresNothing := len(declared) == 0
	ignore := ignoreVariables(p)

	type key struct{ app, variable string }
	groups := map[key][]project.CodeUsage{}
	var order []key
	for _, u := range p.CodeUsages {
		if !u.Certain() || declared[u.Variable] || matchesAny(ignore, u.Variable) {
			continue
		}
		k := key{u.AppRoot, u.Variable}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], u)
	}

	var out []findings.Finding
	for _, k := range order {
		us := groups[k]
		allDefault := true
		for _, u := range us {
			allDefault = allDefault && u.HasDefault
		}
		if scanner.IsWellKnown(k.variable, allDefault) {
			continue
		}
		first := us[0]
		var related []findings.Location
		for _, u := range us[1:min(len(us), 1+maxRelatedUsages)] {
			related = append(related, findings.Location{File: u.File, Line: u.Line, Column: u.Column})
		}
		sev := findings.Error
		why := fmt.Sprintf("Nothing documents that %s must be set, so a new environment or teammate will not provide it "+
			"and the code reads an empty value at runtime.", k.variable)
		switch {
		case allDefault:
			sev = findings.Info
			why = fmt.Sprintf("The code falls back to a default, so nothing breaks if %s is unset, but it is invisible to "+
				"anyone configuring the app and the default may be wrong for production.", k.variable)
		case declaresNothing:
			sev = findings.Info
			why = fmt.Sprintf("This repository declares no environment variables anywhere (no .env files, templates, "+
				"Compose or workflow env entries, or schema), so this is a suggestion to document %s, not an error. "+
				"Without documentation, whoever deploys it has to read the code to find out it exists.", k.variable)
		}
		places := ""
		if len(us) > 1 {
			places = fmt.Sprintf(" (%d places)", len(us))
		}
		target := suggestTemplate(p, k.app)
		fix := fmt.Sprintf("Add `%s=` to %s (read at %s:%d), then set it in each real env file. "+
			"If the platform provides it, add %s to scanner.ignore_variables in .envguard.yaml.",
			k.variable, target, first.File, first.Line, k.variable)
		switch {
		case allDefault:
			fix = fmt.Sprintf("Document it by adding `%s= # optional` to %s (read at %s:%d). "+
				"If you do not want it documented, add %s to scanner.ignore_variables in .envguard.yaml.",
				k.variable, target, first.File, first.Line, k.variable)
		case declaresNothing:
			fix = fmt.Sprintf("Document it by adding `%s=` to %s (read at %s:%d), or `%s= # optional` if the code "+
				"works without it. To silence this, add %s to scanner.ignore_variables in .envguard.yaml.",
				k.variable, target, first.File, first.Line, k.variable, k.variable)
		}
		out = append(out, findings.Finding{
			Severity: sev,
			Message: fmt.Sprintf("%s is read by the code%s but not declared in any env file, template or schema",
				k.variable, places),
			Variable: k.variable,
			File:     first.File,
			Line:     first.Line,
			Column:   first.Column,
			Why:      why,
			Fix:      fix,
			Related:  related,
		})
	}
	return out
}

// declaredEverywhere collects every variable declared anywhere: env files,
// templates, schema variables, Compose definitions and workflow env entries.
func declaredEverywhere(p *project.Project) map[string]bool {
	declared := map[string]bool{}
	for _, f := range p.EnvFiles {
		for _, k := range f.Keys() {
			declared[k] = true
		}
	}
	if p.Settings != nil {
		for k := range p.Settings.Variables {
			declared[k] = true
		}
	}
	if p.Schema != nil {
		for k := range p.Schema.Variables {
			declared[k] = true
		}
	}
	for _, r := range p.ComposeRefs {
		if r.Defines {
			declared[r.Variable] = true
		}
	}
	for _, r := range p.WorkflowRefs {
		if r.Defines {
			declared[r.Variable] = true
		}
	}
	return declared
}

func ignoreVariables(p *project.Project) []string {
	if p.Settings == nil {
		return nil
	}
	return p.Settings.Scanner.IgnoreVariables
}

// suggestTemplate returns where a variable of appRoot should be documented:
// the app's default template, else the nearest enclosing app's default
// template, else a new .env.example in the app root.
func suggestTemplate(p *project.Project, appRoot string) string {
	candidates := []*project.App{p.AppFor(appRoot)}
	candidates = append(candidates, p.AncestorApps(appRoot)...)
	for _, a := range candidates {
		if a == nil {
			continue
		}
		if g := a.Group(discovery.EnvDefault); g != nil && len(g.Templates) > 0 {
			return g.Templates[0].Path
		}
	}
	return joinPath(appRoot, ".env.example")
}
