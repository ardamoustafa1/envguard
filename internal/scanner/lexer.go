package scanner

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"
)

// kind classifies each source byte. Detectors only accept matches that start
// in code, so usages written inside comments or ordinary strings are ignored.
type kind uint8

const (
	kCode kind = iota
	kComment
	kString
)

// quote describes a string literal style.
type quote struct {
	open, close string
	escape      bool // a backslash escapes the next byte
	multiline   bool
	// keepCode consumes the literal as a unit but classifies it as code: shell
	// "..." and `...` expand variables, and consuming them keeps an apostrophe
	// inside them from being mistaken for a single-quoted string.
	keepCode bool
}

type blockComment struct {
	open, close string
	nested      bool
	lineStart   bool // must start at the beginning of a line (Ruby =begin/=end)
}

// syntax is the lexical description of a language family.
type syntax struct {
	line         []string
	hashBoundary bool // '#' starts a comment only at a word boundary (shell)
	block        []blockComment
	quotes       []quote // checked in order: put longer openers first
	jsTemplate   bool    // `...${expr}...` with code inside ${}
	jsRegex      bool    // /regex/ literals
	rustRaw      bool    // r"..." r#"..."# br"..."
	rustChar     bool    // 'c' vs lifetimes 'a
	// regions, if set, limits code to the returned [start, end) ranges; the
	// rest of the file is treated as comment (HTML around <script> or <?php).
	regions func(src []byte) [][2]int
}

// span is a string literal region, recorded for detectors that look inside
// strings on purpose (Go struct tags).
type span struct {
	start, end int // including delimiters
	open       string
}

func lex(src []byte, sx *syntax) (kinds []kind, spans []span) {
	kinds = make([]kind, len(src))
	if sx.regions == nil {
		lexRange(src, 0, len(src), sx, kinds, &spans)
		return kinds, spans
	}
	for i := range kinds {
		kinds[i] = kComment
	}
	for _, r := range sx.regions(src) {
		lexRange(src, r[0], r[1], sx, kinds, &spans)
	}
	return kinds, spans
}

func fill(kinds []kind, from, to int, k kind) {
	for j := from; j < to; j++ {
		kinds[j] = k
	}
}

func lexRange(src []byte, a, b int, sx *syntax, kinds []kind, spans *[]span) {
	fill(kinds, a, b, kCode)
	var tpl []int // brace depth per open ${ in JS template literals
	for i := a; i < b; {
		c := src[i]
		if len(tpl) > 0 {
			top := len(tpl) - 1
			switch c {
			case '{':
				tpl[top]++
			case '}':
				if tpl[top] == 0 {
					tpl = tpl[:top]
					end, interp := scanTemplate(src, i+1, b)
					fill(kinds, i, end, kString)
					if interp {
						tpl = append(tpl, 0)
					}
					i = end
					continue
				}
				tpl[top]--
			}
		}
		if end, ok := sx.lineComment(src, a, b, i); ok {
			fill(kinds, i, end, kComment)
			i = end
			continue
		}
		if end, ok := sx.blockComment(src, a, b, i); ok {
			fill(kinds, i, end, kComment)
			i = end
			continue
		}
		if sx.jsTemplate && c == '`' {
			end, interp := scanTemplate(src, i+1, b)
			fill(kinds, i, end, kString)
			if interp {
				tpl = append(tpl, 0)
			}
			i = end
			continue
		}
		if sx.rustRaw && (c == 'r' || c == 'b') {
			if end, ok := rustRaw(src, a, b, i); ok {
				fill(kinds, i, end, kString)
				i = end
				continue
			}
		}
		if sx.rustChar && c == '\'' {
			if end, ok := rustChar(src, b, i); ok {
				fill(kinds, i, end, kString)
				i = end
			} else {
				i++ // a lifetime such as 'a
			}
			continue
		}
		if sx.jsRegex && c == '/' && regexAllowed(src, kinds, a, i) {
			if end, ok := scanRegex(src, b, i); ok {
				fill(kinds, i, end, kString)
				i = end
				continue
			}
		}
		if q := sx.quoteAt(src, i); q != nil {
			end := scanQuote(src, b, i, q)
			if !q.keepCode {
				fill(kinds, i, end, kString)
				*spans = append(*spans, span{start: i, end: end, open: q.open})
			}
			i = end
			continue
		}
		i++
	}
}

func (sx *syntax) lineComment(src []byte, a, b, i int) (int, bool) {
	for _, lc := range sx.line {
		if !bytes.HasPrefix(src[i:b], []byte(lc)) {
			continue
		}
		if sx.hashBoundary && lc == "#" && i > a && !strings.ContainsRune(" \t\n;(|&", rune(src[i-1])) {
			continue
		}
		return lineEnd(src, b, i), true
	}
	return 0, false
}

func (sx *syntax) blockComment(src []byte, a, b, i int) (int, bool) {
	for _, bc := range sx.block {
		if !bytes.HasPrefix(src[i:b], []byte(bc.open)) {
			continue
		}
		if bc.lineStart {
			if i > a && src[i-1] != '\n' {
				continue
			}
			idx := bytes.Index(src[i:b], []byte("\n"+bc.close))
			if idx < 0 {
				return b, true
			}
			return lineEnd(src, b, i+idx+1), true
		}
		if !bc.nested {
			idx := bytes.Index(src[i+len(bc.open):b], []byte(bc.close))
			if idx < 0 {
				return b, true
			}
			return i + len(bc.open) + idx + len(bc.close), true
		}
		depth := 0
		for j := i; j < b; {
			switch {
			case bytes.HasPrefix(src[j:b], []byte(bc.open)):
				depth++
				j += len(bc.open)
			case bytes.HasPrefix(src[j:b], []byte(bc.close)):
				depth--
				j += len(bc.close)
				if depth == 0 {
					return j, true
				}
			default:
				j++
			}
		}
		return b, true
	}
	return 0, false
}

func (sx *syntax) quoteAt(src []byte, i int) *quote {
	for k := range sx.quotes {
		if bytes.HasPrefix(src[i:], []byte(sx.quotes[k].open)) {
			return &sx.quotes[k]
		}
	}
	return nil
}

func scanQuote(src []byte, b, i int, q *quote) int {
	j := i + len(q.open)
	for j < b {
		if q.escape && src[j] == '\\' {
			j += 2
			continue
		}
		if bytes.HasPrefix(src[j:b], []byte(q.close)) {
			return j + len(q.close)
		}
		if !q.multiline && src[j] == '\n' {
			return j
		}
		j++
	}
	return b
}

// scanTemplate scans a JS template literal body from j. It stops after the
// closing backtick (interp=false) or right after "${" (interp=true).
func scanTemplate(src []byte, j, b int) (end int, interp bool) {
	for j < b {
		switch {
		case src[j] == '\\':
			j += 2
		case src[j] == '`':
			return j + 1, false
		case src[j] == '$' && j+1 < b && src[j+1] == '{':
			return j + 2, true
		default:
			j++
		}
	}
	return b, false
}

func rustRaw(src []byte, a, b, i int) (int, bool) {
	if i > a && isIdentByte(src[i-1]) {
		return 0, false
	}
	j := i
	if src[j] == 'b' {
		j++
	}
	if j >= b || src[j] != 'r' {
		return 0, false
	}
	k := j + 1
	hashes := 0
	for k < b && src[k] == '#' {
		hashes++
		k++
	}
	if k >= b || src[k] != '"' {
		return 0, false
	}
	closing := []byte("\"" + strings.Repeat("#", hashes))
	idx := bytes.Index(src[k+1:b], closing)
	if idx < 0 {
		return b, true
	}
	return k + 1 + idx + len(closing), true
}

// rustChar recognizes 'c' and '\n' style char literals; anything else is a
// lifetime and stays code.
func rustChar(src []byte, b, i int) (int, bool) {
	if i+1 < b && src[i+1] == '\\' {
		limit := min(b, i+12)
		if k := bytes.IndexByte(src[i+2:limit], '\''); k >= 0 {
			return i + 2 + k + 1, true
		}
		return 0, false
	}
	_, size := utf8.DecodeRune(src[i+1 : b])
	if size > 0 && i+1+size < b && src[i+1+size] == '\'' {
		return i + 2 + size, true
	}
	return 0, false
}

var regexKeywords = map[string]bool{
	"return": true, "typeof": true, "case": true, "do": true, "else": true, "in": true,
	"of": true, "new": true, "delete": true, "void": true, "throw": true, "yield": true, "await": true,
}

// regexAllowed decides whether '/' at i starts a regex literal (rather than
// division) from the previous significant code byte.
func regexAllowed(src []byte, kinds []kind, a, i int) bool {
	if i+1 < len(src) && (src[i+1] == '/' || src[i+1] == '*') {
		return false
	}
	p := i - 1
	for p >= a && (src[p] == ' ' || src[p] == '\t' || src[p] == '\n' || src[p] == '\r' || kinds[p] == kComment) {
		p--
	}
	if p < a {
		return true
	}
	if strings.ContainsRune("(,=:[!&|?{};+-*%<>~^", rune(src[p])) {
		return true
	}
	if isIdentByte(src[p]) {
		s := p
		for s > a && isIdentByte(src[s-1]) {
			s--
		}
		return regexKeywords[string(src[s:p+1])]
	}
	return false
}

func scanRegex(src []byte, b, i int) (int, bool) {
	inClass := false
	for j := i + 1; j < b; j++ {
		switch src[j] {
		case '\\':
			j++
		case '\n':
			return 0, false
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				j++
				for j < b && isIdentByte(src[j]) {
					j++
				}
				return j, true
			}
		}
	}
	return 0, false
}

func lineEnd(src []byte, b, i int) int {
	if k := bytes.IndexByte(src[i:b], '\n'); k >= 0 {
		return i + k
	}
	return b
}

func isIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

var (
	scriptTagRE = regexp.MustCompile(`(?is)<script\b[^>]*>(.*?)</script\s*>`)
	phpOpenRE   = regexp.MustCompile(`<\?(?:php\b|=)`)
)

// scriptRegions returns the bodies of <script> elements (Vue, Svelte).
func scriptRegions(src []byte) [][2]int {
	var out [][2]int
	for _, m := range scriptTagRE.FindAllSubmatchIndex(src, -1) {
		out = append(out, [2]int{m[2], m[3]})
	}
	return out
}

// phpRegions returns <?php ... ?> and <?= ... ?> blocks; text outside them
// is output, not code.
func phpRegions(src []byte) [][2]int {
	var out [][2]int
	for i := 0; i < len(src); {
		loc := phpOpenRE.FindIndex(src[i:])
		if loc == nil {
			break
		}
		start := i + loc[1]
		end := len(src)
		if k := bytes.Index(src[start:], []byte("?>")); k >= 0 {
			end = start + k
		}
		out = append(out, [2]int{start, end})
		i = end
	}
	return out
}
