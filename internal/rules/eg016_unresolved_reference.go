package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(unresolvedReference{base{"EG016", "unresolved-reference", findings.Warning}}) }

// unresolvedReference reports $VAR / ${VAR} references inside env files
// that nothing defines. A reference resolves if the variable is defined in
// the same file, in any real env file or template of the app's default
// environment or of the file's own environment (loaders such as Next.js,
// Vite and docker compose load .env together with .env.<env>), is a
// well-known process variable (HOME, PATH, ...), or is listed in
// scanner.ignore_variables. References with a default (${VAR:-x}) never
// trigger.
type unresolvedReference struct{ base }

// wellKnownVars are provided by the operating system or shell.
var wellKnownVars = map[string]bool{
	"HOME": true, "USER": true, "USERNAME": true, "LOGNAME": true, "PATH": true, "PWD": true, "SHELL": true,
	"HOSTNAME": true, "TMPDIR": true, "TMP": true, "TEMP": true, "LANG": true, "TERM": true,
	"USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
}

func (unresolvedReference) Check(p *project.Project) []findings.Finding {
	var ignore []string
	if p.Settings != nil {
		ignore = p.Settings.Scanner.IgnoreVariables
	}
	var out []findings.Finding
	for _, app := range p.Apps {
		for _, g := range app.Groups {
			if !p.EnvSelected(g.Environment) {
				continue
			}
			defined := map[string]bool{}
			var related []string
			envs := []string{discovery.EnvDefault}
			if g.Environment != discovery.EnvDefault {
				envs = append(envs, g.Environment)
			}
			for _, env := range envs {
				grp := app.Group(env)
				if grp == nil {
					continue
				}
				for _, f := range append(append([]*project.EnvFile{}, grp.Files...), grp.Templates...) {
					for _, k := range f.Keys() {
						defined[k] = true
					}
					related = append(related, f.Path)
				}
			}
			for _, f := range append(append([]*project.EnvFile{}, g.Files...), g.Templates...) {
				for _, e := range f.Entries() {
					for _, r := range e.References {
						if r.HasDefault || defined[r.Name] || wellKnownVars[r.Name] || matchesAny(ignore, r.Name) {
							continue
						}
						out = append(out, findings.Finding{
							Message: fmt.Sprintf("%s references %s, which is not defined in %s or the env files loaded with it",
								e.Key, r.Name, f.Path),
							Variable: r.Name,
							File:     f.Path,
							Line:     r.Pos.Line,
							Column:   r.Pos.Column,
							Why: fmt.Sprintf("Loaders that expand references substitute an empty string for an undefined %s, "+
								"so %s silently ends up incomplete (for example a URL without a host).", r.Name, e.Key),
							Fix: fmt.Sprintf("Define `%s=` in %s (checked: %s), give the reference a default such as "+
								"`${%s:-value}`, or add %s to scanner.ignore_variables in .envguard.yaml if the "+
								"process environment provides it.", r.Name, f.Path, joinList(related), r.Name, r.Name),
						})
					}
				}
			}
		}
	}
	return out
}

func matchesAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if findings.MatchName(p, name) {
			return true
		}
	}
	return false
}
