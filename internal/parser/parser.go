package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

const (
	utf8BOM   = "\xef\xbb\xbf"
	exportKw  = "export"
	dirIgnore = "envguard-ignore"
	dirFile   = "envguard-ignore-file"
)

// Hints. They are fixed strings: never include input text in them.
const (
	hintKey           = "Keys may contain only ASCII letters, digits, '_', '.' and '-', and must not start with a digit."
	hintMissingEquals = "Write assignments as KEY=value. Keys cannot contain spaces, " +
		"and a bare 'export KEY' without a value is not supported."
	hintEmptyKey     = "Add a variable name before '=', e.g. KEY=value."
	hintUnterminated = "Add the matching closing quote. Only double quotes and backticks may span multiple lines; " +
		"the quote opened here was never closed."
	hintWhitespace = "Wrap the value in double quotes. Some loaders cut unquoted values at the first space " +
		"or treat the rest as a separate token."
	hintEscape = "Supported escapes in double quotes are \\n \\t \\r \\\" \\\\ and \\$. " +
		"Use \\\\ for a literal backslash, or single quotes for fully literal text."
	hintTrailing  = "Remove the text after the closing quote, or start an inline comment with '#'."
	hintDirective = "Use '# envguard-ignore EG002,EG004' (next entry) or '# envguard-ignore-file EG002' " +
		"(whole file) with rule IDs of the form EG000."
	hintTooLarge = "Env files are expected to be small. Split the file or exclude it from the scan."
)

// Parse reads a dotenv file from r. The returned error is non-nil only for I/O
// failures; malformed input is reported as Problems on the returned File.
// Parse never panics.
func Parse(r io.Reader, filename string) (*File, error) {
	if r == nil {
		return nil, fmt.Errorf("read %s: %w", filename, errors.New("nil reader"))
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}
	f := &File{Filename: filename}
	if len(data) > MaxFileSize {
		f.Problems = append(f.Problems, Problem{
			Kind:     ProblemFileTooLarge,
			Severity: SeverityError,
			Pos:      Pos{Line: 1, Column: 1},
			Message:  "file exceeds the 1 MiB size limit and was not parsed",
			Hint:     hintTooLarge,
		})
		return f, nil
	}

	bom := 0
	if bytes.HasPrefix(data, []byte(utf8BOM)) {
		bom = len(utf8BOM)
	}
	p := &parser{src: data[bom:], f: f}
	p.pt = newPosTable(p.src, bom)
	p.run()

	slices.SortStableFunc(f.Problems, func(a, b Problem) int { return a.Pos.Offset - b.Pos.Offset })
	f.FileIgnoreRules = sortedUnique(f.FileIgnoreRules)
	return f, nil
}

type parser struct {
	src     []byte
	pt      posTable
	f       *File
	pending []int // indices into f.Directives awaiting their target line
}

func (p *parser) run() {
	n := len(p.src)
	for i := 0; i < n; {
		j := p.skipBlanks(i)
		switch {
		case j >= n:
			return
		case isEOL(p.src[j]):
			i = p.nextLine(j)
		case p.src[j] == '#':
			end := p.lineEnd(j)
			p.directive(j, end, nil)
			i = p.nextLine(end)
		default:
			i = p.assignment(j)
		}
	}
}

// assignment parses a KEY=VALUE line starting at start (first non-blank byte)
// and returns the offset of the next line to parse.
func (p *parser) assignment(start int) int {
	src, n := p.src, len(p.src)
	ignore := p.takePending(start)

	pos := start
	hasExport := false
	if hasPrefix(src[pos:], exportKw) && pos+len(exportKw) < n && isBlank(src[pos+len(exportKw)]) {
		k := p.skipBlanks(pos + len(exportKw))
		if k < n && src[k] != '=' && !isEOL(src[k]) {
			hasExport = true
			pos = k
		}
	}

	keyStart := pos
	for pos < n && src[pos] != '=' && !isBlank(src[pos]) && !isEOL(src[pos]) {
		pos++
	}
	key := src[keyStart:pos]
	eq := p.skipBlanks(pos)

	if eq >= n || src[eq] != '=' {
		p.problem(ProblemMissingEquals, SeverityError, keyStart, "", "missing '=' after key", hintMissingEquals)
		return p.nextLine(p.lineEnd(eq))
	}
	if len(key) == 0 {
		p.problem(ProblemEmptyKey, SeverityError, eq, "", "missing key before '='", hintEmptyKey)
		return p.nextLine(p.lineEnd(eq))
	}
	if bad, msg := invalidKey(key); bad >= 0 {
		p.problem(ProblemInvalidKey, SeverityError, keyStart+bad, "", msg, hintKey)
		return p.nextLine(p.lineEnd(eq))
	}

	e := Entry{
		Key:         string(key),
		KeyPos:      p.pos(keyStart),
		HasExport:   hasExport,
		Quote:       QuoteNone,
		IgnoreRules: ignore,
	}

	vs := p.skipBlanks(eq + 1)
	hadBlank := vs > eq+1
	var next int
	switch {
	case vs >= n || isEOL(src[vs]):
		e.ValuePos = p.pos(eq + 1)
		next = p.nextLine(vs)
	case src[vs] == '#' && hadBlank:
		e.ValuePos = p.pos(eq + 1)
		end := p.lineEnd(vs)
		p.inlineComment(&e, vs, end)
		next = p.nextLine(end)
	case src[vs] == '\'' || src[vs] == '"' || src[vs] == '`':
		var ok bool
		next, ok = p.quoted(&e, vs)
		if !ok {
			return next
		}
	default:
		next = p.unquoted(&e, vs)
	}

	e.IgnoreRules = sortedUnique(e.IgnoreRules)
	p.f.Entries = append(p.f.Entries, e)
	return next
}

func (p *parser) unquoted(e *Entry, vs int) int {
	src := p.src
	end := p.lineEnd(vs)
	valEnd, hash := end, -1
	for k := vs + 1; k < end; k++ {
		if src[k] == '#' && isBlank(src[k-1]) {
			valEnd, hash = k, k
			break
		}
	}
	for valEnd > vs && isBlank(src[valEnd-1]) {
		valEnd--
	}
	raw := src[vs:valEnd]
	e.Value = string(raw)
	e.RawValue = e.Value
	e.ValuePos = p.pos(vs)
	e.References = p.refs(vs, valEnd, false)
	if ws := unbracedBlank(raw); ws >= 0 {
		p.problem(ProblemUnquotedWhitespace, SeverityWarning, vs+ws, e.Key,
			"unquoted value contains whitespace", hintWhitespace)
	}
	if hash >= 0 {
		p.inlineComment(e, hash, end)
	}
	return p.nextLine(end)
}

// quoted parses a quoted value whose opening quote is at vs. ok is false when
// the quote is unterminated; next is then the line after the opening quote.
func (p *parser) quoted(e *Entry, vs int) (next int, ok bool) {
	src := p.src
	q := src[vs]
	var closing int
	switch q {
	case '\'':
		end := p.lineEnd(vs)
		c := bytes.IndexByte(src[vs+1:end], '\'')
		if c < 0 {
			return p.unterminated(e, vs, "unterminated single quote"), false
		}
		closing = vs + 1 + c
		e.Quote = QuoteSingle
		e.Value = string(src[vs+1 : closing])
		e.ValuePos = p.pos(vs)
	case '`':
		c := bytes.IndexByte(src[vs+1:], '`')
		if c < 0 {
			return p.unterminated(e, vs, "unterminated backtick quote"), false
		}
		closing = vs + 1 + c
		e.Quote = QuoteBacktick
		e.Value = normalizeNewlines(src[vs+1 : closing])
		e.ValuePos = p.pos(vs)
	default: // '"'
		value, c, badEscapes := p.scanDouble(vs + 1)
		if c < 0 {
			return p.unterminated(e, vs, "unterminated double quote"), false
		}
		closing = c
		e.Quote = QuoteDouble
		e.Value = value
		e.ValuePos = p.pos(vs)
		e.References = p.refs(vs+1, closing, true)
		for _, off := range badEscapes {
			p.problem(ProblemInvalidEscape, SeverityError, off, e.Key,
				"invalid escape sequence in double-quoted value", hintEscape)
		}
	}
	e.RawValue = string(src[vs : closing+1])

	k := p.skipBlanks(closing + 1)
	end := p.lineEnd(k)
	if k < end {
		if src[k] == '#' {
			p.inlineComment(e, k, end)
		} else {
			p.problem(ProblemTrailingCharacters, SeverityError, k, e.Key,
				"unexpected characters after closing quote", hintTrailing)
		}
	}
	return p.nextLine(end), true
}

func (p *parser) unterminated(e *Entry, vs int, msg string) int {
	p.problem(ProblemUnterminatedQuote, SeverityError, vs, e.Key, msg, hintUnterminated)
	return p.nextLine(p.lineEnd(vs))
}

// scanDouble scans a double-quoted body starting at start. It returns the
// unescaped value, the offset of the closing quote (-1 if unterminated) and the
// offsets of invalid escape sequences.
func (p *parser) scanDouble(start int) (value string, closing int, badEscapes []int) {
	src, n := p.src, len(p.src)
	var b strings.Builder
	for k := start; k < n; {
		switch ch := src[k]; ch {
		case '"':
			return b.String(), k, badEscapes
		case '\\':
			if k+1 >= n {
				k++
				continue
			}
			switch nx := src[k+1]; nx {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"', '\\', '$':
				b.WriteByte(nx)
			default:
				badEscapes = append(badEscapes, k)
				b.WriteByte('\\')
				if isEOL(nx) {
					k++ // let the newline be handled (and normalized) below
					continue
				}
				b.WriteByte(nx)
			}
			k += 2
		case '\r':
			b.WriteByte('\n')
			if k+1 < n && src[k+1] == '\n' {
				k++
			}
			k++
		default:
			b.WriteByte(ch)
			k++
		}
	}
	return "", -1, nil
}

func (p *parser) inlineComment(e *Entry, hash, end int) {
	e.InlineComment = strings.Trim(string(p.src[hash+1:end]), " \t")
	p.directive(hash, end, e)
}

func (p *parser) problem(kind ProblemKind, sev Severity, off int, key, msg, hint string) {
	p.f.Problems = append(p.f.Problems, Problem{
		Kind:     kind,
		Severity: sev,
		Pos:      p.pos(off),
		Key:      key,
		Message:  msg,
		Hint:     hint,
	})
}

func (p *parser) pos(off int) Pos { return p.pt.pos(off) }

func (p *parser) skipBlanks(i int) int {
	for i < len(p.src) && isBlank(p.src[i]) {
		i++
	}
	return i
}

// lineEnd returns the offset of the line terminator at or after i, or len(src).
func (p *parser) lineEnd(i int) int {
	for i < len(p.src) && !isEOL(p.src[i]) {
		i++
	}
	return i
}

// nextLine returns the offset after the line terminator at i (CRLF, LF or CR).
func (p *parser) nextLine(i int) int {
	n := len(p.src)
	if i >= n {
		return n
	}
	if p.src[i] == '\r' && i+1 < n && p.src[i+1] == '\n' {
		return i + 2
	}
	return i + 1
}

func isBlank(b byte) bool { return b == ' ' || b == '\t' }
func isEOL(b byte) bool   { return b == '\n' || b == '\r' }

func hasPrefix(b []byte, s string) bool {
	return len(b) >= len(s) && string(b[:len(s)]) == s
}

func isKeyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-'
}

// invalidKey returns the index of the first offending byte and a message, or
// -1 if the key is valid.
func invalidKey(key []byte) (idx int, msg string) {
	if key[0] >= '0' && key[0] <= '9' {
		return 0, "key must not start with a digit"
	}
	for i, c := range key {
		if !isKeyByte(c) {
			return i, "key contains an invalid character"
		}
	}
	return -1, ""
}

// unbracedBlank returns the index of the first space or tab that is not inside
// a ${...} expression (e.g. ${VAR:?must be set} is fine), or -1.
func unbracedBlank(b []byte) int {
	depth := 0
	for i, c := range b {
		switch {
		case c == '{' && i > 0 && b[i-1] == '$':
			depth++
		case c == '}' && depth > 0:
			depth--
		case isBlank(c) && depth == 0:
			return i
		}
	}
	return -1
}

func normalizeNewlines(b []byte) string {
	if bytes.IndexByte(b, '\r') < 0 {
		return string(b)
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func sortedUnique(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	slices.Sort(s)
	return slices.Compact(s)
}
