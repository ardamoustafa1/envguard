package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// GitHub writes GitHub Actions workflow commands (annotations), one per
// finding, followed by a plain summary line:
//
//	::error file=.env.production,line=12,col=1,title=EG001 missing-variable::DATABASE_URL is …
func GitHub(w io.Writer, rep *Report) error {
	rep = rep.printable()
	var b strings.Builder
	for i := range rep.Findings {
		f := &rep.Findings[i]
		cmd := "notice"
		switch f.Severity {
		case findings.Error:
			cmd = "error"
		case findings.Warning:
			cmd = "warning"
		}
		props := []string{"file=" + escapeProperty(rep.PathPrefix+f.File)}
		if f.Line > 0 {
			props = append(props, fmt.Sprintf("line=%d", f.Line))
			if f.Column > 0 {
				props = append(props, fmt.Sprintf("col=%d", f.Column))
			}
		}
		props = append(props, "title="+escapeProperty(f.RuleID+" "+f.RuleName))
		msg := f.Message + "\n\nWhy: " + f.Why + "\n\nFix: " + f.Fix
		fmt.Fprintf(&b, "::%s %s::%s\n", cmd, strings.Join(props, ","), escapeData(msg))
	}
	c := findings.Count(rep.Findings)
	fmt.Fprintf(&b, "EnvGuard: %s, %s, %d info in %s", plural(c.Error, "error", "errors"),
		plural(c.Warning, "warning", "warnings"), c.Info, plural(rep.Stats.Files(), "file", "files"))
	if rep.Suppressed > 0 {
		fmt.Fprintf(&b, ", %d suppressed", rep.Suppressed)
	}
	b.WriteString(".\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// escapeData escapes a workflow command message.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProperty escapes a workflow command property value.
func escapeProperty(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}

// GitHubSummary writes a Markdown job summary (for $GITHUB_STEP_SUMMARY).
func GitHubSummary(w io.Writer, rep *Report) error {
	rep = rep.printable()
	var b strings.Builder
	c := findings.Count(rep.Findings)
	b.WriteString("## EnvGuard\n\n")
	if len(rep.Findings) == 0 {
		fmt.Fprintf(&b, "✅ **No problems found** in %s.\n", plural(rep.Stats.Files(), "file", "files"))
	} else {
		fmt.Fprintf(&b, "| ❌ Errors | ⚠️ Warnings | ℹ️ Info | Suppressed | Files |\n|---:|---:|---:|---:|---:|\n| %d | %d | %d | %d | %d |\n\n",
			c.Error, c.Warning, c.Info, rep.Suppressed, rep.Stats.Files())
		b.WriteString("| Severity | Rule | Location | Problem |\n|---|---|---|---|\n")
		const maxRows = 100
		fs := bySeverity(rep.Findings)
		for i := range fs {
			if i == maxRows {
				fmt.Fprintf(&b, "\n…and %d more. Run `envguard doctor` locally for the full report.\n", len(fs)-maxRows)
				break
			}
			f := &fs[i]
			loc := f.Location()
			loc.File = rep.PathPrefix + loc.File
			fmt.Fprintf(&b, "| %s | [%s](%s) %s | `%s` | %s |\n", severityEmoji(f.Severity), f.RuleID, f.DocsURL,
				mdCell(f.RuleName), mdCode(loc.String()), mdCell(f.Message))
		}
		b.WriteString("\nRun `envguard doctor` locally to see why each problem matters and how to fix it.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func severityEmoji(s findings.Severity) string {
	switch s {
	case findings.Error:
		return "❌ error"
	case findings.Warning:
		return "⚠️ warning"
	default:
		return "ℹ️ info"
	}
}

// mdCell escapes text for a Markdown table cell; HTML is neutralized.
func mdCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;", "&", "&amp;").Replace(s)
}

func mdCode(s string) string {
	return strings.NewReplacer("`", "'", "|", "\\|", "\n", " ").Replace(s)
}
