package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/scanner"
)

func init() { Register(declaredButUnused{base{"EG004", "declared-but-unused", findings.Info}}) }

// declaredButUnused reports template variables that nothing reads: no code
// usage anywhere in the repository, no ${VAR} reference in an env file, no
// Compose or workflow reference. App roots where the scanner found no usage
// at all (in the app or below it) are skipped: their language is probably
// not supported, and every variable would look unused (see
// SkippedForUnused). Well-known platform variables and
// scanner.ignore_variables are never reported. One finding per app root and
// variable, at its first template declaration.
type declaredButUnused struct{ base }

func (declaredButUnused) Check(p *project.Project) []findings.Finding {
	if _, bulk := BulkEnvironmentRead(p); bulk {
		return nil
	}
	used := usedEverywhere(p)
	ignore := ignoreVariables(p)
	var out []findings.Finding
	for _, app := range p.Apps {
		if len(p.UsagesUnder(app.Root)) == 0 {
			continue
		}
		dynamic := p.DynamicUsages(app.Root)
		decls, order := declarations(app.Templates())
		for _, key := range order {
			if used[key] || scanner.IsWellKnown(key, true) || matchesAny(ignore, key) {
				continue
			}
			d := decls[key]
			why := "Unused variables make templates longer and send people looking for values nothing needs; " +
				"they are often left over after a refactor."
			if dynamic > 0 {
				lookups := fmt.Sprintf("%d dynamic lookups", dynamic)
				if dynamic == 1 {
					lookups = "1 dynamic lookup"
				}
				why += fmt.Sprintf(" Note: %s in this app (a variable name computed at run time) could not be analyzed and may read it.", lookups)
			}
			out = append(out, findings.Finding{
				Message: fmt.Sprintf("%s is declared in %s but never read by the code, env files, Compose files or workflows",
					key, d.File.Path),
				Variable: key,
				File:     d.File.Path,
				Line:     d.Entry.KeyPos.Line,
				Column:   d.Entry.KeyPos.Column,
				Why:      why,
				Fix: fmt.Sprintf("If nothing needs %s, remove it from %s and from the real env files. "+
					"If a tool outside the scanned code reads it (Prisma, Docker, a framework), add %s to "+
					"scanner.ignore_variables in .envguard.yaml.", key, d.ref(), key),
			})
		}
	}
	return out
}

// usedEverywhere collects every variable that something reads.
func usedEverywhere(p *project.Project) map[string]bool {
	used := map[string]bool{}
	for _, u := range p.CodeUsages {
		if !u.Dynamic {
			used[u.Variable] = true
		}
	}
	for _, f := range p.EnvFiles {
		for _, e := range f.Entries() {
			for _, r := range e.References {
				used[r.Name] = true
			}
		}
	}
	// Compose environment: entries count as used (they pass the variable to
	// a container), as do interpolations.
	for _, r := range p.ComposeRefs {
		used[r.Variable] = true
	}
	for _, r := range p.WorkflowRefs {
		used[r.Variable] = true
	}
	return used
}

// BulkEnvironmentRead reports the first place where the code hands the whole
// environment to a function (schema.parse(process.env), createEnv({ runtimeEnv:
// process.env })). The variable names then live in a schema, not in
// process.env.X reads, so EG004 cannot tell which declared variables are
// unused and stays silent in the whole repository (for verbose output too).
func BulkEnvironmentRead(p *project.Project) (loc findings.Location, found bool) {
	for _, u := range p.CodeUsages {
		if u.Bulk {
			return findings.Location{File: u.File, Line: u.Line, Column: u.Column}, true
		}
	}
	return findings.Location{}, false
}

// SkippedForUnused lists app roots with templates where EG004 does not run
// because the scanner found no env var usage in them (for verbose output).
// (It does not report the repository-wide skip, see BulkEnvironmentRead.)
func SkippedForUnused(p *project.Project) []string {
	var out []string
	for _, app := range p.Apps {
		if len(app.Templates()) > 0 && len(p.UsagesUnder(app.Root)) == 0 {
			out = append(out, app.Root)
		}
	}
	return out
}
