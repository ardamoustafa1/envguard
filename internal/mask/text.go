package mask

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Printable makes text safe to show on a terminal or in a log. File names,
// directory names and YAML keys come from the repository being analyzed, so
// they can contain escape sequences that clear the screen, set the window
// title or fake output, bidirectional overrides that make "txt.exe" look
// like "exe.txt", and bytes that are not UTF-8. Every such character is
// replaced by a visible escape (\x1b, \u202e, \xff). Newlines and tabs are
// kept; callers that need one line collapse them separately.
func Printable(s string) string {
	if !needsEscape(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case unsafeRune(r):
			if r < 0x80 {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

func needsEscape(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || unsafeRune(r) {
			return true
		}
		i += size
	}
	return false
}

// unsafeRune reports control characters (C0, DEL, C1), format characters
// (bidi controls, zero-width characters, BOM) and line/paragraph
// separators. \n and \t are allowed.
func unsafeRune(r rune) bool {
	if r == '\n' || r == '\t' {
		return false
	}
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Bidi_Control, r) ||
		r == '\u2028' || r == '\u2029'
}

// Clip shortens s to at most limit characters, ending in "…" when it was
// cut, so one absurdly long name cannot blow up aligned output.
func Clip(s string, limit int) string {
	if limit < 2 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	n := 0
	for i := range s {
		if n == limit-1 {
			return s[:i] + "…"
		}
		n++
	}
	return s
}
