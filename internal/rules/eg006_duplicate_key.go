package rules

import (
	"fmt"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/parser"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(duplicateKey{base{"EG006", "duplicate-key", findings.Warning}}) }

// duplicateKey reports every repeated definition of a key within one env
// file or template, at the repeated line, with the other definitions as
// Related locations. Evidence shows the masked values when they differ.
type duplicateKey struct{ base }

func (duplicateKey) Check(p *project.Project) []findings.Finding {
	var out []findings.Finding
	for _, f := range p.EnvFiles {
		if !p.EnvSelected(f.Environment) {
			continue
		}
		byKey := map[string][]parser.Entry{}
		var order []string
		for _, e := range f.Entries() {
			if _, ok := byKey[e.Key]; !ok {
				order = append(order, e.Key)
			}
			byKey[e.Key] = append(byKey[e.Key], e)
		}
		for _, key := range order {
			defs := byKey[key]
			if len(defs) < 2 {
				continue
			}
			lines := make([]string, len(defs))
			for i, d := range defs {
				lines[i] = fmt.Sprint(d.KeyPos.Line)
			}
			evidence := duplicateEvidence(p, key, defs)
			for i, d := range defs[1:] {
				var related []findings.Location
				for j, other := range defs {
					if j != i+1 {
						related = append(related, entryLocation(f, other.KeyPos))
					}
				}
				out = append(out, findings.Finding{
					Message: fmt.Sprintf("%s is defined %d times in %s (lines %s)",
						key, len(defs), f.Path, joinList(lines)),
					Variable: key,
					File:     f.Path,
					Line:     d.KeyPos.Line,
					Column:   d.KeyPos.Column,
					Evidence: evidence,
					Why: "Most loaders (dotenv, python-dotenv, Docker) silently use the last definition, " +
						"so editing the first one has no effect and the file says two different things.",
					Fix: fmt.Sprintf("Keep one definition of %s in %s: delete line %d or the earlier one at line %d, "+
						"keeping the value you intend.", key, f.Path, d.KeyPos.Line, defs[0].KeyPos.Line),
					Related: related,
				})
			}
		}
	}
	return out
}

// duplicateEvidence lists masked values per line when they differ.
func duplicateEvidence(p *project.Project, key string, defs []parser.Entry) string {
	same := true
	for _, d := range defs[1:] {
		if d.Value != defs[0].Value {
			same = false
		}
	}
	if same {
		return "all definitions have the same value"
	}
	parts := make([]string, len(defs))
	for i, d := range defs {
		parts[i] = fmt.Sprintf("line %d: %s", d.KeyPos.Line, p.DisplayValue(key, d.Value))
	}
	return strings.Join(parts, "; ")
}
