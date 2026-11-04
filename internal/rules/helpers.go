package rules

import (
	"fmt"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/parser"
	"github.com/ardamoustafa1/envguard/internal/project"
)

// base implements ID, Name and DefaultSeverity for rules.
type base struct {
	id, name string
	severity findings.Severity
}

func (b base) ID() string                         { return b.id }
func (b base) Name() string                       { return b.name }
func (b base) DefaultSeverity() findings.Severity { return b.severity }

// decl is the first declaration of a variable in a set of templates.
type decl struct {
	File  *project.EnvFile
	Entry parser.Entry
}

func (d decl) ref() string { return fmt.Sprintf("%s:%d", d.File.Path, d.Entry.KeyPos.Line) }

func (d decl) location() findings.Location {
	return findings.Location{File: d.File.Path, Line: d.Entry.KeyPos.Line, Column: d.Entry.KeyPos.Column}
}

// declarations returns the first declaration of each key across templates,
// plus the keys in declaration order.
func declarations(templates []*project.EnvFile) (decls map[string]decl, order []string) {
	decls = map[string]decl{}
	for _, t := range templates {
		for _, e := range t.Entries() {
			if _, ok := decls[e.Key]; !ok {
				decls[e.Key] = decl{File: t, Entry: e}
				order = append(order, e.Key)
			}
		}
	}
	return decls, order
}

// isOptionalComment reports whether an inline comment marks a template line
// as optional: "# optional" (case-insensitive), optionally followed by more
// text such as "# optional: defaults to 5".
func isOptionalComment(c string) bool {
	c = strings.ToLower(strings.TrimSpace(c))
	return c == "optional" || strings.HasPrefix(c, "optional:") || strings.HasPrefix(c, "optional ") ||
		strings.HasPrefix(c, "optional,") || strings.HasPrefix(c, "optional -")
}

// IsRequired implements the template convention: a declared variable is
// required if the template gives it a non-empty value, or if its line is not
// marked "# optional". So only `KEY= # optional` declares an optional variable.
func IsRequired(e parser.Entry) bool {
	return e.Value != "" || !isOptionalComment(e.InlineComment)
}

func entryLocation(f *project.EnvFile, pos parser.Pos) findings.Location {
	return findings.Location{File: f.Path, Line: pos.Line, Column: pos.Column}
}

func paths(files []*project.EnvFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// joinList joins names as "a", "a and b" or "a, b and c".
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// templateFor suggests where a variable of env should be documented: that
// environment's own template, else the default template, else a new
// .env.example in the app root.
func templateFor(app *project.App, env string) string {
	if g := app.Group(env); g != nil && len(g.Templates) > 0 {
		return g.Templates[0].Path
	}
	if g := app.Group("default"); g != nil && len(g.Templates) > 0 {
		return g.Templates[0].Path
	}
	return joinPath(app.Root, ".env.example")
}

// where names an env file in messages: its path, or "the process
// environment" in --process-env mode.
func where(f *project.EnvFile) string {
	if f.Path == project.ProcessEnvPath {
		return "the process environment"
	}
	return f.Path
}

func joinPath(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}
