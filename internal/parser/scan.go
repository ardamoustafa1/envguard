package parser

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// refs finds variable references in src[start:end]. When escapes is true
// (double-quoted values) a backslash escapes the following byte, so \$VAR is
// not a reference. "$$" is treated as a literal dollar sign.
//
// For ${VAR<op>word} only VAR is reported; references nested inside word are
// not, because they are only evaluated when VAR is unset or empty.
func (p *parser) refs(start, end int, escapes bool) []Reference {
	src := p.src
	var out []Reference
	var braces map[int]int // lazily built: offset of '{' -> offset of matching '}'
	for i := start; i < end; {
		c := src[i]
		if escapes && c == '\\' {
			i += 2
			continue
		}
		if c != '$' || i+1 >= end {
			i++
			continue
		}
		switch nb := src[i+1]; {
		case nb == '$':
			i += 2
		case nb == '{':
			ref, next, ok := p.bracedRef(i, end, func() map[int]int {
				if braces == nil {
					braces = p.matchBraces(start, end, escapes)
				}
				return braces
			})
			if ok {
				out = append(out, ref)
				i = next
			} else {
				i += 2
			}
		case isNameStart(nb):
			j := i + 2
			for j < end && isNameByte(src[j]) {
				j++
			}
			out = append(out, Reference{Name: string(src[i+1 : j]), Pos: p.pos(i)})
			i = j
		default:
			i++
		}
	}
	return out
}

// bracedRef parses ${NAME}, ${NAME<op>word} with op one of :- - := = :? ? :+ +
// starting at the '$' at i. braces returns the brace-matching table for the
// current value; it is only needed (and built) for the operator forms.
func (p *parser) bracedRef(i, end int, braces func() map[int]int) (ref Reference, next int, ok bool) {
	src := p.src
	j := i + 2
	if j >= end || !isNameStart(src[j]) {
		return Reference{}, 0, false
	}
	for j < end && isNameByte(src[j]) {
		j++
	}
	name := string(src[i+2 : j])
	if j >= end {
		return Reference{}, 0, false
	}
	if src[j] == '}' {
		return Reference{Name: name, Pos: p.pos(i), Braced: true}, j + 1, true
	}
	if src[j] == ':' {
		j++
	}
	if j >= end || !strings.ContainsRune("-=?+", rune(src[j])) {
		return Reference{}, 0, false
	}
	op := src[j]
	closing, found := braces()[i+1]
	if !found {
		return Reference{}, 0, false
	}
	return Reference{
		Name:       name,
		Pos:        p.pos(i),
		Braced:     true,
		HasDefault: op == '-' || op == '=',
	}, closing + 1, true
}

// matchBraces pairs every '{' in src[start:end] with its matching '}' in one
// linear pass (unmatched braces are absent from the result). Doing this once
// per value keeps reference scanning linear even for inputs like
// "${A:-${A:-${A:-..." that never close.
func (p *parser) matchBraces(start, end int, escapes bool) map[int]int {
	src := p.src
	match := make(map[int]int)
	var stack []int
	for k := start; k < end; k++ {
		switch src[k] {
		case '\\':
			if escapes {
				k++
			}
		case '{':
			stack = append(stack, k)
		case '}':
			if n := len(stack); n > 0 {
				match[stack[n-1]] = k
				stack = stack[:n-1]
			}
		}
	}
	return match
}

func isNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isNameByte(c byte) bool  { return isNameStart(c) || c >= '0' && c <= '9' }

// directive handles a comment starting at hash ('#') and ending at end. If e
// is non-nil the comment is an inline comment of e.
func (p *parser) directive(hash, end int, e *Entry) {
	src := p.src
	k := p.skipBlanks(hash + 1)
	kwStart := k
	for k < end && !isBlank(src[k]) {
		k++
	}
	var scope DirectiveScope
	switch string(src[kwStart:k]) {
	case dirIgnore:
		scope = ScopeEntry
	case dirFile:
		scope = ScopeFile
	default:
		return
	}

	listEnd := end
	if idx := strings.Index(string(src[k:end]), "--"); idx >= 0 {
		listEnd = k + idx
	}
	var rules []string
	invalid := false
	for k < listEnd {
		if isBlank(src[k]) || src[k] == ',' {
			k++
			continue
		}
		s := k
		for k < listEnd && !isBlank(src[k]) && src[k] != ',' {
			k++
		}
		id := strings.ToUpper(string(src[s:k]))
		if !isRuleID(id) {
			invalid = true
			p.problem(ProblemInvalidDirective, SeverityWarning, s, "",
				"invalid rule ID in envguard-ignore directive", hintDirective)
			continue
		}
		rules = append(rules, id)
	}
	if len(rules) == 0 {
		if !invalid {
			p.problem(ProblemInvalidDirective, SeverityWarning, hash, "",
				"envguard-ignore directive lists no rule IDs", hintDirective)
		}
		return
	}

	d := Directive{Pos: p.pos(hash), Scope: scope, Rules: sortedUnique(rules)}
	switch {
	case scope == ScopeFile:
		p.f.FileIgnoreRules = append(p.f.FileIgnoreRules, d.Rules...)
	case e != nil:
		d.TargetLine = e.KeyPos.Line
		e.IgnoreRules = append(e.IgnoreRules, d.Rules...)
	default:
		p.pending = append(p.pending, len(p.f.Directives))
	}
	p.f.Directives = append(p.f.Directives, d)
}

// takePending binds pending entry-scoped directives to the assignment line
// starting at off and returns their rule IDs.
func (p *parser) takePending(off int) []string {
	if len(p.pending) == 0 {
		return nil
	}
	line := p.pos(off).Line
	var rules []string
	for _, idx := range p.pending {
		p.f.Directives[idx].TargetLine = line
		rules = append(rules, p.f.Directives[idx].Rules...)
	}
	p.pending = p.pending[:0]
	return rules
}

func isRuleID(s string) bool {
	if len(s) != 5 || s[:2] != "EG" {
		return false
	}
	for _, c := range []byte(s[2:]) {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// posTable converts byte offsets into line/column positions. Lines end at
// "\r\n", "\n" or a lone "\r". It caches the last lookup so that the mostly
// monotonic lookups during parsing are linear overall, even on very long
// lines.
type posTable struct {
	src    []byte
	bom    int
	starts []int

	cOff, cLine, cCol int
}

func newPosTable(src []byte, bom int) posTable {
	starts := []int{0}
	for i, c := range src {
		switch c {
		case '\n':
			starts = append(starts, i+1)
		case '\r':
			if i+1 < len(src) && src[i+1] == '\n' {
				continue
			}
			starts = append(starts, i+1)
		}
	}
	return posTable{src: src, bom: bom, starts: starts, cLine: 1, cCol: 1}
}

func (t *posTable) pos(off int) Pos {
	off = min(max(off, 0), len(t.src))
	line := sort.Search(len(t.starts), func(i int) bool { return t.starts[i] > off })
	var col int
	if line == t.cLine && off >= t.cOff {
		col = t.cCol + utf8.RuneCount(t.src[t.cOff:off])
	} else {
		col = 1 + utf8.RuneCount(t.src[t.starts[line-1]:off])
	}
	t.cOff, t.cLine, t.cCol = off, line, col
	return Pos{Line: line, Column: col, Offset: off + t.bom}
}
