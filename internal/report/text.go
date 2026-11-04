package report

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// step is one numbered fix step: prose plus an optional code block.
type step struct {
	Text string
	Code []string
}

var numberedRE = regexp.MustCompile(`^\s*(\d+)[.)]\s+`)

// fixSteps splits a Fix text into steps. Explicitly numbered lines
// ("1. ...") are kept as steps; otherwise every sentence of a paragraph is
// a step. Indented paragraphs are code and attach to the preceding step.
func fixSteps(fix string) []step {
	fix = strings.ReplaceAll(strings.ReplaceAll(fix, "\r\n", "\n"), "\t", "    ")
	lines := strings.Split(fix, "\n")
	numbered := false
	for _, l := range lines {
		if numberedRE.MatchString(l) {
			numbered = true
			break
		}
	}
	var steps []step
	appendCode := func(code []string) {
		if len(steps) == 0 {
			steps = append(steps, step{})
		}
		last := &steps[len(steps)-1]
		last.Code = append(last.Code, code...)
	}
	if numbered {
		for _, l := range lines {
			switch {
			case numberedRE.MatchString(l):
				steps = append(steps, step{Text: strings.TrimSpace(numberedRE.ReplaceAllString(l, ""))})
			case strings.TrimSpace(l) == "":
			case isCodeLine(l):
				appendCode([]string{l})
			case len(steps) > 0 && len(steps[len(steps)-1].Code) == 0:
				steps[len(steps)-1].Text += " " + strings.TrimSpace(l)
			default:
				steps = append(steps, step{Text: strings.TrimSpace(l)})
			}
		}
		return steps
	}
	for _, para := range strings.Split(fix, "\n\n") {
		pl := strings.Split(strings.Trim(para, "\n"), "\n")
		if len(pl) == 0 || strings.TrimSpace(para) == "" {
			continue
		}
		if allCode(pl) {
			appendCode(dedent(pl))
			continue
		}
		joined := strings.Join(trimAll(pl), " ")
		for _, sentence := range sentences(joined) {
			steps = append(steps, step{Text: sentence})
		}
	}
	return steps
}

func isCodeLine(l string) bool { return strings.HasPrefix(l, "    ") }

func allCode(ls []string) bool {
	for _, l := range ls {
		if strings.TrimSpace(l) != "" && !isCodeLine(l) {
			return false
		}
	}
	return true
}

func dedent(ls []string) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = strings.TrimPrefix(l, "    ")
	}
	return out
}

func trimAll(ls []string) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = strings.TrimSpace(l)
	}
	return out
}

// sentences splits prose at ". ", "! " and "? " followed by an upper-case
// letter, never inside `code`.
func sentences(s string) []string {
	var out []string
	inCode := false
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '`' {
			inCode = !inCode
			continue
		}
		if inCode || (c != '.' && c != '!' && c != '?') || i+2 >= len(s) || s[i+1] != ' ' {
			continue
		}
		r, _ := utf8.DecodeRuneInString(s[i+2:])
		if !unicode.IsUpper(r) && r != '`' {
			continue
		}
		out = append(out, strings.TrimSpace(s[start:i+1]))
		start = i + 2
	}
	if rest := strings.TrimSpace(s[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

// wrap word-wraps prose to width runes. The first line is not indented;
// following lines get indent. Words longer than width are not broken.
func wrap(text string, width int, indent string) string {
	if width < 20 {
		width = 20
	}
	var b strings.Builder
	col := 0
	for i, word := range words(text) {
		wl := utf8.RuneCountInString(word)
		if i > 0 {
			if col+1+wl > width {
				b.WriteString("\n" + indent)
				col = 0
			} else {
				b.WriteByte(' ')
				col++
			}
		}
		b.WriteString(word)
		col += wl
	}
	return b.String()
}

// words splits on blanks, but keeps `code spans` in one piece so a span is
// never broken across lines.
func words(text string) []string {
	var out []string
	var cur strings.Builder
	inCode := false
	for _, r := range text {
		switch {
		case r == '`':
			inCode = !inCode
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !inCode:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		case unicode.IsSpace(r):
			cur.WriteByte(' ')
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
