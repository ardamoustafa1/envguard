package rules

import (
	"fmt"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(undocumentedVariable{base{"EG002", "undocumented-variable", findings.Warning}}) }

// undocumentedVariable reports keys of real env files that no template of
// the same app root (or of an enclosing app root, e.g. a shared root
// .env.example in a monorepo) documents. When there is no template at all,
// it reports the file once instead of every variable.
type undocumentedVariable struct{ base }

const maxListedKeys = 10

func (undocumentedVariable) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	for _, app := range p.Apps {
		templates := app.Templates()
		for _, anc := range p.AncestorApps(app.Root) {
			templates = append(templates, anc.Templates()...)
		}
		documented := map[string]bool{}
		for _, t := range templates {
			for _, k := range t.Keys() {
				documented[k] = true
			}
		}

		for _, g := range app.Groups {
			if !p.EnvSelected(g.Environment) {
				continue
			}
			for _, f := range g.Files {
				keys := f.Keys()
				if len(keys) == 0 {
					continue
				}
				if len(templates) == 0 && p.Schema == nil {
					out = append(out, noTemplate(app, f, keys))
					continue
				}
				target := templateFor(app, g.Environment)
				for _, key := range keys {
					if documented[key] || schemaDocumented(p, key) {
						continue
					}
					e, _ := f.Lookup(key)
					out = append(out, findings.Finding{
						Message:  fmt.Sprintf("%s is set in %s but not documented in any template", key, f.Path),
						Variable: key,
						File:     f.Path,
						Line:     e.KeyPos.Line,
						Column:   e.KeyPos.Column,
						Why: fmt.Sprintf("Teammates, CI and deploy targets learn which variables exist from the templates; "+
							"without an entry, %s will be missing wherever this file is not copied.", key),
						Fix: fmt.Sprintf("Add `%s=` (with an empty or placeholder value, never the real one) to %s, "+
							"or remove it from %s if nothing uses it.", key, target, f.Path),
					})
				}
			}
		}
	}
	return out
}

func noTemplate(app *project.App, f *project.EnvFile, keys []string) findings.Finding {
	target := joinPath(app.Root, ".env.example")
	shown := keys
	if len(shown) > maxListedKeys {
		shown = shown[:maxListedKeys]
	}
	var snippet strings.Builder
	for _, k := range shown {
		fmt.Fprintf(&snippet, "    %s=\n", k)
	}
	if len(keys) > len(shown) {
		fmt.Fprintf(&snippet, "    # ... and %d more\n", len(keys)-len(shown))
	}
	noun := "variables"
	if len(keys) == 1 {
		noun = "variable"
	}
	return findings.Finding{
		Message: fmt.Sprintf("%s defines %d %s but no template documents them", f.Path, len(keys), noun),
		File:    f.Path,
		Why: "Without a template such as .env.example nobody can tell which variables the app needs, " +
			"so new environments are set up by guesswork.",
		Fix: fmt.Sprintf("Create %s listing every variable without its real value:\n\n%s", target, snippet.String()),
	}
}
