package rules

import (
	"fmt"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/mask"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() {
	Register(possibleSecretInTemplate{base{"EG010", "possible-secret-in-template", findings.Error}})
}

// withArticle prefixes "a" or "an": "an AWS access key ID", "a URL with ...".
func withArticle(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune("AEIOaeio", rune(s[0])) || (s[0] == 'U' && !strings.HasPrefix(s, "URL")) {
		return "an " + s
	}
	return "a " + s
}

// possibleSecretInTemplate reports values that look like real credentials
// (mask.LooksLikeSecret) in templates, which are meant to be committed, and
// in real env files that git tracks. Placeholders such as `your-api-key`,
// `changeme` or `<token>` never trigger. The message names the detected kind;
// the value is never shown.
type possibleSecretInTemplate struct{ base }

func (possibleSecretInTemplate) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	gitOK := p.Git != nil && p.Git.Available
	for _, f := range p.EnvFiles {
		if !p.EnvSelected(f.Environment) {
			continue
		}
		tracked := gitOK && p.Git.Tracked[f.Path]
		if !f.IsTemplate() && !tracked {
			continue
		}
		where := "template"
		if !f.IsTemplate() {
			where = "committed env file"
		}
		for _, e := range f.Entries() {
			kind, ok := mask.LooksLikeSecret(e.Key, e.Value)
			if !ok {
				continue
			}
			out = append(out, findings.Finding{
				Message:  fmt.Sprintf("%s in %s looks like %s", e.Key, f.Path, withArticle(kind)),
				Variable: e.Key,
				File:     f.Path,
				Line:     e.ValuePos.Line,
				Column:   e.ValuePos.Column,
				Evidence: "detected: " + kind,
				Why: fmt.Sprintf("A %s is shared with everyone who can read the repository; a real credential there "+
					"is leaked, and it stays in git history even after it is removed.", where),
				Fix: fmt.Sprintf("Replace the value with an empty one or a placeholder (`%s=` or `%s=<your-value>`). "+
					"If it is a real credential, rotate it now: it has been exposed. If it is a deliberate fake, use an "+
					"obvious placeholder or add `# envguard-ignore EG010` on the line above.", e.Key, e.Key),
			})
		}
	}
	return out
}
