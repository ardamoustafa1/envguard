package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// Pretty writes the compact `check` format: findings grouped by file,
//
//	.env.production
//	  12:1  ✗ error    EG001  DATABASE_URL is declared in .env.example but missing from .env.production
//
// then a one-line summary such as
//
//	✗ 2 errors · ⚠ 3 warnings · 1 info · 14 files
func Pretty(w io.Writer, rep *Report, opts Options) error {
	rep = rep.printable()
	s := newStyler(opts)
	var b strings.Builder

	fs := rep.Findings
	for start := 0; start < len(fs); {
		end := start
		for end < len(fs) && fs[end].File == fs[start].File {
			end++
		}
		group := fs[start:end]
		locWidth := 0
		for i := range group {
			locWidth = max(locWidth, len(lineCol(&group[i])))
		}
		b.WriteString(s.paint(s.bold, displayFile(group[0].File)))
		b.WriteByte('\n')
		for i := range group {
			f := &group[i]
			loc := lineCol(f)
			b.WriteString("  ")
			if locWidth > 0 {
				b.WriteString(padLeft(loc, s.paint(s.dim, loc), locWidth))
				b.WriteString("  ")
			}
			b.WriteString(s.severity(f.Severity))
			b.WriteString("  ")
			b.WriteString(s.paint(s.dim, f.RuleID))
			b.WriteString("  ")
			b.WriteString(s.highlight(oneLine(f.Message), f.Variable))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
		start = end
	}

	b.WriteString(summary(s, rep, opts))
	b.WriteByte('\n')
	if len(fs) > 0 {
		hint := fmt.Sprintf("Run `envguard doctor` to see why each problem matters and how to fix it, or `envguard explain %s`.",
			bySeverity(fs)[0].RuleID)
		b.WriteString(s.paint(s.dim, hint))
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, opts.finish(b.String()))
	return err
}

func summary(s *styler, rep *Report, opts Options) string {
	var parts []string
	if len(rep.Findings) == 0 {
		parts = append(parts, s.paint(s.green, s.sym.OK+" No problems found"))
	} else {
		parts = append(parts, s.counts(findings.Count(rep.Findings)))
	}
	if rep.ProcessEnv {
		parts = append(parts, "process environment")
	} else {
		parts = append(parts, plural(rep.Stats.Files(), "file", "files"))
	}
	if rep.Suppressed > 0 {
		parts = append(parts, fmt.Sprintf("%d suppressed", rep.Suppressed))
	}
	if opts.ShowDuration {
		parts = append(parts, formatDuration(rep.Duration))
	}
	return strings.Join(parts, s.sep())
}

func formatDuration(d time.Duration) string {
	if d < 10*time.Millisecond {
		return fmt.Sprintf("%.3fs", d.Seconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// lineCol is "12:1", "12" or "" for whole-file findings.
func lineCol(f *findings.Finding) string {
	switch {
	case f.Line <= 0:
		return ""
	case f.Column <= 0:
		return fmt.Sprint(f.Line)
	default:
		return fmt.Sprintf("%d:%d", f.Line, f.Column)
	}
}

func displayFile(file string) string {
	if file == "" {
		return "(repository)"
	}
	return file
}
