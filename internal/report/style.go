package report

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// symbols are the glyphs used by the human formats.
type symbols struct {
	Error, Warning, Info, OK  string
	Sep, Arrow, Bullet, Dash  string
	Set, Empty, Missing, None string
	Rule                      string
}

var unicodeSymbols = symbols{
	Error: "✗", Warning: "⚠", Info: "•", OK: "✓",
	Sep: "·", Arrow: "→", Bullet: "•", Dash: "–",
	Set: "✓", Empty: "○", Missing: "✗", None: "–",
	Rule: "─",
}

var asciiSymbols = symbols{
	Error: "x", Warning: "!", Info: "i", OK: "ok",
	Sep: "|", Arrow: "->", Bullet: "*", Dash: "-",
	Set: "+", Empty: "o", Missing: "x", None: "-",
	Rule: "-",
}

// styler paints text. Without color it returns text unchanged, so no
// escape sequence can ever reach non-terminal output.
type styler struct {
	color bool
	sym   symbols

	bold, dim, faintItalic   lipgloss.Style
	red, yellow, blue, green lipgloss.Style
	cyan, heading            lipgloss.Style
	codeStyle                lipgloss.Style
}

func newStyler(o Options) *styler {
	s := &styler{color: o.Color, sym: unicodeSymbols}
	if o.ASCII {
		s.sym = asciiSymbols
	}
	// The basic 16-color ANSI palette follows the user's terminal theme
	// (light or dark) and works on every terminal, Windows included.
	r := lipgloss.NewRenderer(nil)
	r.SetColorProfile(termenv.ANSI)
	r.SetHasDarkBackground(true)
	s.bold = r.NewStyle().Bold(true)
	s.dim = r.NewStyle().Faint(true)
	s.faintItalic = r.NewStyle().Faint(true).Italic(true)
	s.red = r.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	s.yellow = r.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	s.blue = r.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	s.green = r.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	s.cyan = r.NewStyle().Foreground(lipgloss.Color("6"))
	s.heading = r.NewStyle().Bold(true).Underline(true)
	s.codeStyle = r.NewStyle().Foreground(lipgloss.Color("6"))
	return s
}

func (s *styler) paint(st lipgloss.Style, text string) string {
	if !s.color || text == "" {
		return text
	}
	return st.Render(text)
}

func (s *styler) sevStyle(sev findings.Severity) lipgloss.Style {
	switch sev {
	case findings.Error:
		return s.red
	case findings.Warning:
		return s.yellow
	default:
		return s.blue
	}
}

func (s *styler) sevSymbol(sev findings.Severity) string {
	switch sev {
	case findings.Error:
		return s.sym.Error
	case findings.Warning:
		return s.sym.Warning
	default:
		return s.sym.Info
	}
}

// severity renders "✗ error" padded to the width of "⚠ warning".
func (s *styler) severity(sev findings.Severity) string {
	text := s.sevSymbol(sev) + " " + sev.String()
	pad := strings.Repeat(" ", max(0, utf8.RuneCountInString(s.sym.Warning+" warning")-utf8.RuneCountInString(text)))
	return s.paint(s.sevStyle(sev), text) + pad
}

// code renders inline `code` spans of a prose string in the code color and
// drops the backticks when colors are on; without colors the backticks stay.
func (s *styler) code(text string) string {
	if !s.color || !strings.Contains(text, "`") {
		return text
	}
	var b strings.Builder
	parts := strings.Split(text, "`")
	if len(parts)%2 == 0 { // unbalanced: leave as is
		return text
	}
	for i, p := range parts {
		if i%2 == 1 {
			b.WriteString(s.paint(s.codeStyle, p))
		} else {
			b.WriteString(p)
		}
	}
	return b.String()
}

// highlight makes the first occurrence of name in msg bold.
func (s *styler) highlight(msg, name string) string {
	if !s.color || name == "" {
		return msg
	}
	i := strings.Index(msg, name)
	if i < 0 {
		return msg
	}
	return msg[:i] + s.paint(s.bold, name) + msg[i+len(name):]
}

// counts renders "✗ 2 errors · ⚠ 3 warnings · 1 info".
func (s *styler) counts(c findings.Counts) string {
	part := func(n int, sev findings.Severity, one, many string) string {
		word := many
		if n == 1 {
			word = one
		}
		text := fmt.Sprintf("%d %s", n, word)
		if sev != findings.Info {
			text = s.sevSymbol(sev) + " " + text
		}
		if n == 0 {
			return s.paint(s.dim, text)
		}
		return s.paint(s.sevStyle(sev), text)
	}
	sep := " " + s.paint(s.dim, s.sym.Sep) + " "
	return part(c.Error, findings.Error, "error", "errors") + sep +
		part(c.Warning, findings.Warning, "warning", "warnings") + sep +
		part(c.Info, findings.Info, "info", "info")
}

func (s *styler) sep() string { return " " + s.paint(s.dim, s.sym.Sep) + " " }

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// padRight pads s (measured in runes, without escape codes) to width.
func padRight(visible, painted string, width int) string {
	return painted + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(visible)))
}

func padLeft(visible, painted string, width int) string {
	return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(visible))) + painted
}

// oneLine guards one-line formats against embedded newlines.
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

var asciiReplacer = strings.NewReplacer("→", "->", "←", "<-", "—", "-", "–", "-", "…", "...", "×", "x",
	"‘", "'", "’", "'", "“", "\"", "”", "\"", "•", "*", "·", "|", "✓", "ok", "✗", "x", "⚠", "!", "─", "-")

// finish applies the ASCII transliteration to a whole output (rule texts
// may contain arrows or dashes) when Options.ASCII is set.
func (o Options) finish(text string) string {
	if !o.ASCII {
		return text
	}
	return asciiReplacer.Replace(text)
}
