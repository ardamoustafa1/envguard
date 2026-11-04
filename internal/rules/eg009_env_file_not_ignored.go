package rules

import (
	"fmt"
	"path"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(envFileNotIgnored{base{"EG009", "env-file-not-ignored", findings.Warning}}) }

// envFileNotIgnored reports real env files that exist, are not tracked, and
// are not covered by any .gitignore rule (according to git check-ignore):
// the next `git add .` would commit them. Needs git; silent otherwise.
type envFileNotIgnored struct{ base }

func (envFileNotIgnored) Check(p *project.Project) []findings.Finding {
	if p.Git == nil || !p.Git.Available {
		return nil
	}
	var out []findings.Finding
	for _, f := range p.EnvFiles {
		if f.IsTemplate() || p.Git.Tracked[f.Path] || p.Git.Ignored[f.Path] || !p.EnvSelected(f.Environment) {
			continue
		}
		gitignore := joinPath(path.Dir(f.Path), ".gitignore")
		out = append(out, findings.Finding{
			Message: fmt.Sprintf("%s is not ignored by git; `git add .` would commit it", f.Path),
			File:    f.Path,
			Why: "Real env files hold credentials. One `git add .` or `git commit -a` away from being committed, " +
				"they end up in history, forks and CI logs.",
			Fix: fmt.Sprintf("Add `%s` to %s (or add `.env*` and `!*.example` to the root .gitignore to cover every env "+
				"file except templates), then check with `git check-ignore -v %s`.", path.Base(f.Path), gitignore, f.Path),
		})
	}
	return out
}
