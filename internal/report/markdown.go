package report

import (
	"io"
	"regexp"
	"strings"
)

var (
	boldRE   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	bulletRE = regexp.MustCompile(`^(\s*)[-*]\s+`)
	olRE     = regexp.MustCompile(`^(\s*)(\d+)\.\s+`)
)

// Markdown renders a rule document for the terminal: headings, wrapped
// paragraphs, lists and indented code blocks. With colors, emphasis and
// code are styled and their markers removed; without colors the text keeps
// backticks so it stays unambiguous.
func Markdown(w io.Writer, md string, opts Options) error {
	s := newStyler(opts)
	width := opts.width()
	var b strings.Builder
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var para []string
	flush := func() {
		if len(para) == 0 {
			return
		}
		b.WriteString("  " + s.inline(wrap(strings.Join(para, " "), width-2, "  ")) + "\n")
		para = nil
	}
	inFence := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "```"):
			flush()
			inFence = !inFence
			if !inFence {
				b.WriteString("\n")
			}
		case inFence:
			b.WriteString("      " + s.paint(s.codeStyle, l) + "\n")
		case strings.HasPrefix(t, "# "):
			flush()
			title := strings.TrimPrefix(t, "# ")
			b.WriteString(s.paint(s.bold, title) + "\n")
			b.WriteString(s.paint(s.dim, strings.Repeat(s.sym.Rule, min(width, len([]rune(title))))) + "\n")
		case strings.HasPrefix(t, "## ") || strings.HasPrefix(t, "### "):
			flush()
			b.WriteString(s.paint(s.heading, strings.TrimLeft(t, "# ")) + "\n")
		case t == "":
			flush()
			b.WriteString("\n")
		case strings.HasPrefix(t, "|"):
			flush()
			b.WriteString("  " + s.inline(t) + "\n")
		case bulletRE.MatchString(l):
			flush()
			m := bulletRE.FindStringSubmatch(l)
			item := l[len(m[0]):]
			for i+1 < len(lines) && continues(lines[i+1]) {
				i++
				item += " " + strings.TrimSpace(lines[i])
			}
			ind := "  " + m[1]
			b.WriteString(ind + s.paint(s.dim, s.sym.Bullet) + " " + s.inline(wrap(item, width-len(ind)-2, ind+"  ")) + "\n")
		case olRE.MatchString(l):
			flush()
			m := olRE.FindStringSubmatch(l)
			item := l[len(m[0]):]
			for i+1 < len(lines) && continues(lines[i+1]) {
				i++
				item += " " + strings.TrimSpace(lines[i])
			}
			ind := "  " + m[1]
			num := m[2] + ". "
			b.WriteString(ind + num + s.inline(wrap(item, width-len(ind)-len(num), ind+strings.Repeat(" ", len(num)))) + "\n")
		default:
			para = append(para, t)
		}
	}
	flush()
	out := strings.TrimRight(trimRight(b.String()), "\n") + "\n"
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	_, err := io.WriteString(w, opts.finish(out))
	return err
}

// continues reports whether a line continues the preceding list item.
func continues(l string) bool {
	t := strings.TrimSpace(l)
	return t != "" && strings.HasPrefix(l, "  ") && !bulletRE.MatchString(l) && !olRE.MatchString(l) &&
		!strings.HasPrefix(t, "```")
}

// inline styles **bold** and `code` per line.
func (s *styler) inline(text string) string {
	return paintLines(text, func(l string) string {
		if !s.color {
			return boldRE.ReplaceAllString(l, "$1")
		}
		l = boldRE.ReplaceAllStringFunc(l, func(m string) string { return s.paint(s.bold, m[2:len(m)-2]) })
		return s.code(l)
	})
}

// RuleList writes `envguard explain --list`.
func RuleList(w io.Writer, rules []RuleInfo, opts Options) error {
	s := newStyler(opts)
	nameW := 0
	for _, r := range rules {
		nameW = max(nameW, len(r.Name))
	}
	var b strings.Builder
	b.WriteString(s.paint(s.dim, padRight("RULE", "RULE", 7)+padRight("NAME", "NAME", nameW+2)+padRight("SEVERITY", "SEVERITY", 10)+"DESCRIPTION") + "\n")
	descIndent := strings.Repeat(" ", 7+nameW+2+10)
	for _, r := range rules {
		sev := r.Severity.String()
		b.WriteString(s.paint(s.bold, r.ID) + "  " + padRight(r.Name, r.Name, nameW+2) +
			padRight(sev, s.paint(s.sevStyle(r.Severity), sev), 10) +
			s.code(wrap(r.Summary, max(30, opts.width()-len(descIndent)), descIndent)) + "\n")
	}
	b.WriteString("\n" + s.code("Run `envguard explain <RULE>` for details, e.g. `envguard explain EG001`.") + "\n")
	_, err := io.WriteString(w, opts.finish(b.String()))
	return err
}
