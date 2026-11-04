package rules

import (
	"fmt"
	"path"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(envFileCommitted{base{"EG008", "env-file-committed", findings.Error}}) }

// envFileCommitted reports real (non-template) env files that git tracks.
// It needs git: when git is unavailable or the root is not a repository the
// rule reports nothing (check --verbose says why).
type envFileCommitted struct{ base }

func (envFileCommitted) Check(p *project.Project) []findings.Finding {
	if p.Git == nil || !p.Git.Available {
		return nil
	}
	var out []findings.Finding
	for _, f := range p.EnvFiles {
		if f.IsTemplate() || !p.Git.Tracked[f.Path] || !p.EnvSelected(f.Environment) {
			continue
		}
		out = append(out, findings.Finding{
			Message: fmt.Sprintf("%s is a real env file and is committed to git", f.Path),
			File:    f.Path,
			Why: "Everyone with access to the repository, its forks and every clone can read the values, " +
				"and they stay in git history even after the file is deleted.",
			Fix: fmt.Sprintf("1. Stop tracking it (the local file is kept): `git rm --cached %s`\n"+
				"2. Ignore it: add `%s` to .gitignore (or `.env*` plus `!*.example` to cover all env files), then commit.\n"+
				"3. WARNING: the values are still in git history. Rotate every secret in this file (new keys, new passwords); "+
				"deleting the file or rewriting history afterwards is not enough, because clones and forks keep the old commits.",
				f.Path, path.Base(f.Path)),
		})
	}
	return out
}
