// Package docs embeds the rule documentation under docs/rules. It is the
// single source of truth for `envguard explain`, SARIF rule metadata and the
// website.
package docs

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Rules holds docs/rules/*.md. Rule documents are named EGxxx-<rule-name>.md.
//
//go:embed rules/*.md
var Rules embed.FS

// RuleDoc is the parsed documentation of one rule.
type RuleDoc struct {
	// ID is the rule ID, e.g. "EG001".
	ID string
	// Name is the rule name from the title, e.g. "missing-variable".
	Name string
	// File is the file name inside docs/rules.
	File string
	// Severity is the text of the "**Default severity:**" line.
	Severity string
	// Summary is the one-sentence description below the severity line.
	Summary string
	// Markdown is the whole document.
	Markdown string
}

// Section returns the body of the "## <title>" section (without the
// heading), trimmed, or "" if there is none.
func (d RuleDoc) Section(title string) string {
	var b strings.Builder
	in := false
	for line := range strings.SplitSeq(d.Markdown, "\n") {
		if strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = strings.EqualFold(strings.TrimSpace(line[3:]), title)
			continue
		}
		if in {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimSpace(b.String())
}

// Rule returns the documentation of a rule by ID (case-insensitive).
func Rule(id string) (RuleDoc, bool) {
	id = strings.ToUpper(id)
	for _, d := range AllRules() {
		if d.ID == id {
			return d, true
		}
	}
	return RuleDoc{}, false
}

// AllRules returns every rule document, sorted by ID. Files that do not
// follow the EGxxx-<name>.md naming are ignored.
func AllRules() []RuleDoc {
	entries, err := fs.ReadDir(Rules, "rules")
	if err != nil {
		return nil
	}
	var out []RuleDoc
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "EG") || path.Ext(name) != ".md" {
			continue
		}
		data, err := fs.ReadFile(Rules, "rules/"+name)
		if err != nil {
			continue
		}
		out = append(out, parse(name, string(data)))
	}
	slices.SortFunc(out, func(a, b RuleDoc) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func parse(file, md string) RuleDoc {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	d := RuleDoc{File: file, Markdown: md}
	d.ID, _, _ = strings.Cut(strings.TrimSuffix(file, ".md"), "-")
	sawSeverity := false
	for line := range strings.SplitSeq(md, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case strings.HasPrefix(t, "# ") && d.Name == "":
			fields := strings.Fields(t[2:])
			if len(fields) >= 2 {
				d.Name = fields[1]
			}
		case strings.HasPrefix(t, "**Default severity:**") || strings.HasPrefix(t, "**Severity:**"):
			_, v, _ := strings.Cut(t, ":**")
			d.Severity = strings.TrimSpace(v)
			sawSeverity = true
		case sawSeverity && d.Summary == "" && !strings.HasPrefix(t, "#"):
			d.Summary = t
		}
		if d.Summary != "" {
			break
		}
	}
	return d
}
