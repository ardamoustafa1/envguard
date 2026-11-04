package parser

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func mustParse(t testing.TB, input string) *File {
	t.Helper()
	f, err := Parse(strings.NewReader(input), "test.env")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

// privateKeyBlock builds a PEM-shaped block at runtime so no secret-looking
// literal exists in the repository.
func privateKeyBlock(nl string) string {
	label := "PRIVATE" + " " + "KEY"
	body := strings.Repeat("Q", 64)
	return "-----BEGIN " + label + "-----" + nl + body + nl + body[:20] + nl + "-----END " + label + "-----"
}

type wantEntry struct {
	key, value, raw string
	quote           Quote
	keyLine, keyCol int
	valLine, valCol int
	export          bool
	comment         string
}

func TestParseEntries(t *testing.T) {
	longValue := strings.Repeat("x", 200_000)
	pem := privateKeyBlock("\n")

	tests := []struct {
		name  string
		input string
		want  []wantEntry
	}{
		{"simple", "KEY=value", []wantEntry{{key: "KEY", value: "value", raw: "value", quote: QuoteNone, keyLine: 1, keyCol: 1, valLine: 1, valCol: 5}}},
		{"export prefix", "export KEY=v", []wantEntry{{key: "KEY", value: "v", quote: QuoteNone, keyLine: 1, keyCol: 8, valCol: 12, export: true}}},
		{"export with tabs", "export\t\tKEY=v", []wantEntry{{key: "KEY", value: "v", quote: QuoteNone, keyCol: 9, export: true}}},
		{"export as key", "export=1", []wantEntry{{key: "export", value: "1", quote: QuoteNone}}},
		{"export as key with spaces", "export = 1", []wantEntry{{key: "export", value: "1", quote: QuoteNone}}},
		{"whitespace around equals", "KEY  =  value", []wantEntry{{key: "KEY", value: "value", quote: QuoteNone, valCol: 9}}},
		{"tabs around equals", "KEY\t=\tvalue", []wantEntry{{key: "KEY", value: "value", quote: QuoteNone, valCol: 7}}},
		{"indented key", "  \tKEY=v", []wantEntry{{key: "KEY", value: "v", quote: QuoteNone, keyCol: 4}}},
		{"empty value", "KEY=", []wantEntry{{key: "KEY", value: "", raw: "", quote: QuoteNone, valLine: 1, valCol: 5}}},
		{"empty value trailing spaces", "KEY=   ", []wantEntry{{key: "KEY", value: "", quote: QuoteNone, valCol: 5}}},
		{"empty double", `KEY=""`, []wantEntry{{key: "KEY", value: "", raw: `""`, quote: QuoteDouble, valCol: 5}}},
		{"empty single", `KEY=''`, []wantEntry{{key: "KEY", value: "", raw: `''`, quote: QuoteSingle}}},
		{"empty backtick", "KEY=``", []wantEntry{{key: "KEY", value: "", raw: "``", quote: QuoteBacktick}}},
		{"trailing whitespace trimmed", "KEY=value \t ", []wantEntry{{key: "KEY", value: "value", raw: "value", quote: QuoteNone}}},
		{"quoted keeps whitespace", `KEY="  v  "`, []wantEntry{{key: "KEY", value: "  v  ", quote: QuoteDouble}}},
		{"single literal", `KEY='a $B \n ${C}'`, []wantEntry{{key: "KEY", value: `a $B \n ${C}`, raw: `'a $B \n ${C}'`, quote: QuoteSingle}}},
		{"double escapes", `KEY="a\nb\tc\rd\"e\\f\$g"`, []wantEntry{{key: "KEY", value: "a\nb\tc\rd\"e\\f$g", quote: QuoteDouble}}},
		{"double no expansion", `KEY="$HOME"`, []wantEntry{{key: "KEY", value: "$HOME", quote: QuoteDouble}}},
		{"backtick literal", "KEY=`a \\n $B \"c\" 'd'`", []wantEntry{{key: "KEY", value: `a \n $B "c" 'd'`, quote: QuoteBacktick}}},
		{"equals in value", "KEY=a=b=c", []wantEntry{{key: "KEY", value: "a=b=c", quote: QuoteNone}}},
		{"base64 padding", "KEY=ZGVtbw==", []wantEntry{{key: "KEY", value: "ZGVtbw==", quote: QuoteNone}}},
		{"url with query and fragment", "URL=https://example.com/p?a=1&b=2#frag", []wantEntry{{key: "URL", value: "https://example.com/p?a=1&b=2#frag", quote: QuoteNone}}},
		{"hash inside value", "PASS=abc#123", []wantEntry{{key: "PASS", value: "abc#123", quote: QuoteNone}}},
		{"hash right after equals", "KEY=#value", []wantEntry{{key: "KEY", value: "#value", quote: QuoteNone}}},
		{"inline comment", "KEY=value # note", []wantEntry{{key: "KEY", value: "value", raw: "value", quote: QuoteNone, comment: "note"}}},
		{"inline comment after tab", "KEY=value\t#note", []wantEntry{{key: "KEY", value: "value", quote: QuoteNone, comment: "note"}}},
		{"comment only value", "KEY= # note", []wantEntry{{key: "KEY", value: "", raw: "", quote: QuoteNone, comment: "note", valCol: 5}}},
		{"empty comment", "KEY=v #", []wantEntry{{key: "KEY", value: "v", quote: QuoteNone}}},
		{"comment after double", `KEY="a # b" # c`, []wantEntry{{key: "KEY", value: "a # b", raw: `"a # b"`, quote: QuoteDouble, comment: "c"}}},
		{"comment tight after single", `KEY='a'#c`, []wantEntry{{key: "KEY", value: "a", quote: QuoteSingle, comment: "c"}}},
		{"json in single quotes", `JSON='{"a": [1, 2], "b": "c#d"}'`, []wantEntry{{key: "JSON", value: `{"a": [1, 2], "b": "c#d"}`, quote: QuoteSingle}}},
		{"windows path unquoted", `P=C:\Users\me\app`, []wantEntry{{key: "P", value: `C:\Users\me\app`, quote: QuoteNone}}},
		{"windows path single", `P='C:\Program Files\x'`, []wantEntry{{key: "P", value: `C:\Program Files\x`, quote: QuoteSingle}}},
		{"windows path double escaped", `P="C:\\Users\\me"`, []wantEntry{{key: "P", value: `C:\Users\me`, quote: QuoteDouble}}},
		{"quote mid unquoted value", `K=it"s`, []wantEntry{{key: "K", value: `it"s`, quote: QuoteNone}}},
		{"unicode double", `K="héllo wörld ✓"`, []wantEntry{{key: "K", value: "héllo wörld ✓", quote: QuoteDouble}}},
		{"unicode unquoted then key column", "K=日本語\nN=1", []wantEntry{
			{key: "K", value: "日本語", quote: QuoteNone},
			{key: "N", value: "1", quote: QuoteNone, keyLine: 2, keyCol: 1},
		}},
		{"column counts runes", "K=日本 # c\nX=1", []wantEntry{{key: "K", value: "日本", quote: QuoteNone, comment: "c"}, {key: "X", value: "1", quote: QuoteNone}}},
		{"dot and dash keys", "a.b=1\nc-d=2\n_e=3", []wantEntry{
			{key: "a.b", value: "1", quote: QuoteNone},
			{key: "c-d", value: "2", quote: QuoteNone},
			{key: "_e", value: "3", quote: QuoteNone},
		}},
		{"multiline double", "K=\"a\nb\"\nN=1", []wantEntry{
			{key: "K", value: "a\nb", raw: "\"a\nb\"", quote: QuoteDouble},
			{key: "N", value: "1", quote: QuoteNone, keyLine: 3},
		}},
		{"multiline double crlf normalized", "K=\"a\r\nb\"\r\nN=1", []wantEntry{
			{key: "K", value: "a\nb", raw: "\"a\r\nb\"", quote: QuoteDouble},
			{key: "N", value: "1", quote: QuoteNone, keyLine: 3},
		}},
		{"multiline backtick", "K=`a\r\nb\rc`", []wantEntry{{key: "K", value: "a\nb\nc", quote: QuoteBacktick}}},
		{"private key shaped block", "KEY=\"" + pem + "\"\nNEXT=1", []wantEntry{
			{key: "KEY", value: pem, quote: QuoteDouble},
			{key: "NEXT", value: "1", quote: QuoteNone, keyLine: 5},
		}},
		{"private key escaped newlines", "KEY=\"" + strings.ReplaceAll(pem, "\n", `\n`) + "\"", []wantEntry{{key: "KEY", value: pem, quote: QuoteDouble}}},
		{"crlf", "A=1\r\nB=2\r\n", []wantEntry{{key: "A", value: "1", quote: QuoteNone}, {key: "B", value: "2", quote: QuoteNone, keyLine: 2}}},
		{"lone cr", "A=1\rB=2", []wantEntry{{key: "A", value: "1", quote: QuoteNone}, {key: "B", value: "2", quote: QuoteNone, keyLine: 2}}},
		{"mixed endings", "A=1\r\nB=2\nC=3\rD=4", []wantEntry{
			{key: "A", value: "1", quote: QuoteNone},
			{key: "B", value: "2", quote: QuoteNone, keyLine: 2},
			{key: "C", value: "3", quote: QuoteNone, keyLine: 3},
			{key: "D", value: "4", quote: QuoteNone, keyLine: 4},
		}},
		{"bom", "\xef\xbb\xbfK=v", []wantEntry{{key: "K", value: "v", quote: QuoteNone, keyLine: 1, keyCol: 1}}},
		{"no trailing newline", "A=1\nB=2", []wantEntry{{key: "A", value: "1", quote: QuoteNone}, {key: "B", value: "2", quote: QuoteNone}}},
		{"comments and blanks", "# c\n\n   \n\t# c2\nK=v", []wantEntry{{key: "K", value: "v", quote: QuoteNone, keyLine: 5}}},
		{"very long line", "K=" + longValue, []wantEntry{{key: "K", value: longValue, quote: QuoteNone}}},
		{"duplicates kept in order", "D=1\nD=2\nD=3", []wantEntry{
			{key: "D", value: "1", quote: QuoteNone, keyLine: 1},
			{key: "D", value: "2", quote: QuoteNone, keyLine: 2},
			{key: "D", value: "3", quote: QuoteNone, keyLine: 3},
		}},
		{"empty input", "", nil},
		{"only comments", "# a\n# b\n", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := mustParse(t, tt.input)
			if tt.want == nil {
				return
			}
			if len(f.Entries) != len(tt.want) {
				t.Fatalf("got %d entries, want %d (problems: %+v)", len(f.Entries), len(tt.want), f.Problems)
			}
			for i, w := range tt.want {
				checkEntry(t, f.Entries[i], w)
			}
		})
	}
}

func checkEntry(t *testing.T, e Entry, w wantEntry) {
	t.Helper()
	if e.Key != w.key {
		t.Errorf("Key = %q, want %q", e.Key, w.key)
	}
	if e.Value != w.value {
		t.Errorf("%s: Value = %q, want %q", w.key, e.Value, w.value)
	}
	if w.raw != "" && e.RawValue != w.raw {
		t.Errorf("%s: RawValue = %q, want %q", w.key, e.RawValue, w.raw)
	}
	if e.Quote != w.quote {
		t.Errorf("%s: Quote = %q, want %q", w.key, e.Quote, w.quote)
	}
	if e.HasExport != w.export {
		t.Errorf("%s: HasExport = %v, want %v", w.key, e.HasExport, w.export)
	}
	if e.InlineComment != w.comment {
		t.Errorf("%s: InlineComment = %q, want %q", w.key, e.InlineComment, w.comment)
	}
	checkPos(t, w.key+" KeyPos", e.KeyPos, w.keyLine, w.keyCol)
	checkPos(t, w.key+" ValuePos", e.ValuePos, w.valLine, w.valCol)
}

func checkPos(t *testing.T, what string, p Pos, line, col int) {
	t.Helper()
	if line != 0 && p.Line != line {
		t.Errorf("%s line = %d, want %d", what, p.Line, line)
	}
	if col != 0 && p.Column != col {
		t.Errorf("%s column = %d, want %d", what, p.Column, col)
	}
}

func TestParseBOMOffset(t *testing.T) {
	f := mustParse(t, "\xef\xbb\xbfK=v")
	if got := f.Entries[0].KeyPos; got != (Pos{Line: 1, Column: 1, Offset: 3}) {
		t.Errorf("KeyPos = %+v, want offset 3 column 1", got)
	}
}

type wantProblem struct {
	kind      ProblemKind
	sev       Severity
	line, col int
}

func TestParseProblems(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     []wantProblem
		wantKeys []string // keys of the entries that must still be produced
	}{
		{"starts with digit", "1KEY=v", []wantProblem{{ProblemInvalidKey, SeverityError, 1, 1}}, nil},
		{"invalid char", "KEY$=v", []wantProblem{{ProblemInvalidKey, SeverityError, 1, 4}}, nil},
		{"unicode key", "KÉY=v", []wantProblem{{ProblemInvalidKey, SeverityError, 1, 2}}, nil},
		{"missing equals", "KEY", []wantProblem{{ProblemMissingEquals, SeverityError, 1, 1}}, nil},
		{"space in key", "MY KEY=v", []wantProblem{{ProblemMissingEquals, SeverityError, 1, 1}}, nil},
		{"export without value", "export KEY", []wantProblem{{ProblemMissingEquals, SeverityError, 1, 8}}, nil},
		{"bare export", "export", []wantProblem{{ProblemMissingEquals, SeverityError, 1, 1}}, nil},
		{"empty key", "  =v", []wantProblem{{ProblemEmptyKey, SeverityError, 1, 3}}, nil},
		{"unquoted whitespace", "K=hello world", []wantProblem{{ProblemUnquotedWhitespace, SeverityWarning, 1, 8}}, []string{"K"}},
		{"unquoted tab", "K=a\tb", []wantProblem{{ProblemUnquotedWhitespace, SeverityWarning, 1, 4}}, []string{"K"}},
		{"whitespace before comment is fine", "K=a # b c", nil, []string{"K"}},
		{"whitespace inside braces is fine", "K=${A:?must be set}", nil, []string{"K"}},
		{"whitespace after braces warns", "K=${A} b", []wantProblem{{ProblemUnquotedWhitespace, SeverityWarning, 1, 7}}, []string{"K"}},
		{"invalid escape", `K="a\qb"`, []wantProblem{{ProblemInvalidEscape, SeverityError, 1, 5}}, []string{"K"}},
		{"windows path in double quotes", `K="C:\Users"`, []wantProblem{{ProblemInvalidEscape, SeverityError, 1, 6}}, []string{"K"}},
		{"backslash newline", "K=\"a\\\nb\"", []wantProblem{{ProblemInvalidEscape, SeverityError, 1, 5}}, []string{"K"}},
		{"trailing after quote", `K="x" y`, []wantProblem{{ProblemTrailingCharacters, SeverityError, 1, 7}}, []string{"K"}},
		{"single quote cannot escape", `K='it\'s'`, []wantProblem{{ProblemTrailingCharacters, SeverityError, 1, 8}}, []string{"K"}},
		{"unterminated single", "K='abc\nN=1", []wantProblem{{ProblemUnterminatedQuote, SeverityError, 1, 3}}, []string{"N"}},
		{"single does not span lines", "K='a\nb'\nN=1", []wantProblem{
			{ProblemUnterminatedQuote, SeverityError, 1, 3},
			{ProblemMissingEquals, SeverityError, 2, 1},
		}, []string{"N"}},
		{"unterminated double points to opening line", "A=1\nK=\"abc\nN=1", []wantProblem{{ProblemUnterminatedQuote, SeverityError, 2, 3}}, []string{"A", "N"}},
		{"unterminated double at eof after escape", `K="abc\`, []wantProblem{{ProblemUnterminatedQuote, SeverityError, 1, 3}}, nil},
		{"unterminated backtick", "K=`abc\nN=1", []wantProblem{{ProblemUnterminatedQuote, SeverityError, 1, 3}}, []string{"N"}},
		{"directive invalid id", "# envguard-ignore EGX\nK=1", []wantProblem{{ProblemInvalidDirective, SeverityWarning, 1, 19}}, []string{"K"}},
		{"directive without ids", "# envguard-ignore\nK=1", []wantProblem{{ProblemInvalidDirective, SeverityWarning, 1, 1}}, []string{"K"}},
		{"problems sorted by position", "K=\"a\\q\" x", []wantProblem{
			{ProblemInvalidEscape, SeverityError, 1, 5},
			{ProblemTrailingCharacters, SeverityError, 1, 9},
		}, []string{"K"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := mustParse(t, tt.input)
			if len(f.Problems) != len(tt.want) {
				t.Fatalf("got %d problems, want %d: %+v", len(f.Problems), len(tt.want), f.Problems)
			}
			for i, w := range tt.want {
				p := f.Problems[i]
				if p.Kind != w.kind || p.Severity != w.sev || p.Pos.Line != w.line || p.Pos.Column != w.col {
					t.Errorf("problem %d = %s/%s at %d:%d, want %s/%s at %d:%d",
						i, p.Kind, p.Severity, p.Pos.Line, p.Pos.Column, w.kind, w.sev, w.line, w.col)
				}
				if p.Message == "" || p.Hint == "" {
					t.Errorf("problem %d has empty message or hint", i)
				}
			}
			var keys []string
			for _, e := range f.Entries {
				keys = append(keys, e.Key)
			}
			if !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("entries = %v, want %v", keys, tt.wantKeys)
			}
		})
	}
}

func TestParseFileTooLarge(t *testing.T) {
	input := strings.Repeat("K=v\n", MaxFileSize/4+1)
	f := mustParse(t, input)
	if len(f.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(f.Entries))
	}
	if len(f.Problems) != 1 || f.Problems[0].Kind != ProblemFileTooLarge {
		t.Fatalf("problems = %+v, want one file-too-large", f.Problems)
	}

	exact := strings.Repeat("K=v\n", MaxFileSize/4)
	if f := mustParse(t, exact); len(f.Problems) != 0 || len(f.Entries) != MaxFileSize/4 {
		t.Errorf("file of exactly MaxFileSize: %d entries, %d problems", len(f.Entries), len(f.Problems))
	}
}

func TestParseIOErrors(t *testing.T) {
	sentinel := errors.New("disk on fire")
	_, err := Parse(iotest.ErrReader(sentinel), "x.env")
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wrapping %v", err, sentinel)
	}
	if _, err := Parse(nil, "x.env"); err == nil {
		t.Error("nil reader: want error")
	}
}

type wantRef struct {
	name       string
	line, col  int
	braced     bool
	hasDefault bool
}

func TestParseReferences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []wantRef
	}{
		{"bare", "K=$A", []wantRef{{"A", 1, 3, false, false}}},
		{"braced", "K=${A}", []wantRef{{"A", 1, 3, true, false}}},
		{"colon dash default", "K=${A:-x}", []wantRef{{"A", 1, 3, true, true}}},
		{"dash default", "K=${A-x}", []wantRef{{"A", 1, 3, true, true}}},
		{"assign default", "K=${A:=x}", []wantRef{{"A", 1, 3, true, true}}},
		{"required", "K=${A:?err}", []wantRef{{"A", 1, 3, true, false}}},
		{"alternate", "K=${A:+x}", []wantRef{{"A", 1, 3, true, false}}},
		{"empty default", "K=${A:-}", []wantRef{{"A", 1, 3, true, true}}},
		{"nested default reports outer only", "K=${A:-${B}}", []wantRef{{"A", 1, 3, true, true}}},
		{"several in double", `K="pre-${A}-$B_C-end"`, []wantRef{{"A", 1, 8, true, false}, {"B_C", 1, 13, false, false}}},
		{"in url", "U=http://$HOST:${PORT}/x", []wantRef{{"HOST", 1, 10, false, false}, {"PORT", 1, 16, true, false}}},
		{"escaped in double", `K="\$A \${B}"`, nil},
		{"escaped closing brace in default", `K="${A:-x\}y}"`, []wantRef{{"A", 1, 4, true, true}}},
		{"single quoted", "K='$A ${B}'", nil},
		{"backtick", "K=`$A`", nil},
		{"double dollar", "K=$$A", nil},
		{"digit", "K=$1", nil},
		{"trailing dollar", "K=price$", nil},
		{"unclosed brace", "K=${A", nil},
		{"unknown operator", "K=${A/x}", nil},
		{"empty braces", "K=${}", nil},
		{"comment excluded", "K=$A # $B", []wantRef{{"A", 1, 3, false, false}}},
		{"after unicode", `K="日本$B"`, []wantRef{{"B", 1, 6, false, false}}},
		{"multiline position", "K=\"a\n  $B\"", []wantRef{{"B", 2, 3, false, false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := mustParse(t, tt.input)
			if len(f.Entries) != 1 {
				t.Fatalf("got %d entries: %+v", len(f.Entries), f.Problems)
			}
			got := f.Entries[0].References
			if len(got) != len(tt.want) {
				t.Fatalf("got %d refs %+v, want %d", len(got), got, len(tt.want))
			}
			for i, w := range tt.want {
				r := got[i]
				if r.Name != w.name || r.Pos.Line != w.line || r.Pos.Column != w.col || r.Braced != w.braced || r.HasDefault != w.hasDefault {
					t.Errorf("ref %d = %+v, want %+v", i, r, w)
				}
			}
		})
	}
}

func TestParseDirectives(t *testing.T) {
	input := strings.Join([]string{
		"# envguard-ignore-file EG004",  // 1
		"# envguard-ignore EG010,EG002", // 2
		"",                              // 3
		"A=1",                           // 4
		"# envguard-ignore eg013 EG013 -- reason", // 5
		"B=1",                              // 6
		"C=1 # envguard-ignore EG005",      // 7
		"D=1",                              // 8
		"# envguard-ignore EG001",          // 9
		"# a regular comment in between",   // 10
		"# envguard-ignore EG006",          // 11
		"E=1",                              // 12
		"# envguard-ignore EG007",          // 13
		"1BAD=1",                           // 14
		"M=\"# envguard-ignore EG009\nx\"", // 15-16
		"# envguard-ignore EG003",          // 17 (dangling)
	}, "\n")
	f := mustParse(t, input)

	wantRules := map[string][]string{
		"A": {"EG002", "EG010"},
		"B": {"EG013"},
		"C": {"EG005"},
		"D": nil,
		"E": {"EG001", "EG006"},
		"M": nil,
	}
	for _, e := range f.Entries {
		want, ok := wantRules[e.Key]
		if !ok {
			t.Errorf("unexpected entry %q", e.Key)
			continue
		}
		if !slices.Equal(e.IgnoreRules, want) {
			t.Errorf("%s.IgnoreRules = %v, want %v", e.Key, e.IgnoreRules, want)
		}
	}
	if !slices.Equal(f.FileIgnoreRules, []string{"EG004"}) {
		t.Errorf("FileIgnoreRules = %v", f.FileIgnoreRules)
	}

	var targets []int
	for _, d := range f.Directives {
		targets = append(targets, d.TargetLine)
	}
	if want := []int{0, 4, 6, 7, 12, 12, 14, 0}; !slices.Equal(targets, want) {
		t.Errorf("directive target lines = %v, want %v", targets, want)
	}

	checks := []struct {
		rule string
		line int
		want bool
	}{
		{"EG004", 99, true},
		{"EG002", 4, true},
		{"EG002", 6, false},
		{"EG007", 14, true},
		{"EG005", 7, true},
		{"EG003", 0, false},
		{"EG009", 15, false},
	}
	for _, c := range checks {
		if got := f.IsIgnored(c.rule, c.line); got != c.want {
			t.Errorf("IsIgnored(%s, %d) = %v, want %v", c.rule, c.line, got, c.want)
		}
	}
}

// TestProblemsNeverEchoInput guards principle 3: problem text must not carry
// any input text (keys of malformed lines may themselves be secrets).
func TestProblemsNeverEchoInput(t *testing.T) {
	const s = "S3NT1NEL"
	inputs := []string{
		s,
		s + " X=1",
		"1" + s + "=x",
		s + "$=x",
		"=" + s,
		"export " + s,
		"K=" + s + " " + s,
		`K="` + s + `\q"`,
		`K="` + s + `" ` + s,
		`K='` + s,
		`K="` + s,
		"K=`" + s,
		"# envguard-ignore " + s,
		"# envguard-ignore-file " + s,
	}
	for _, in := range inputs {
		f := mustParse(t, in)
		if len(f.Problems) == 0 {
			t.Errorf("%q: expected at least one problem", in)
		}
		for _, p := range f.Problems {
			for _, field := range []string{p.Message, p.Hint, p.Key} {
				if strings.Contains(field, s) {
					t.Errorf("%q: problem %s leaks input text: %q", in, p.Kind, field)
				}
			}
		}
	}
}

// TestParsePathologicalInputs guards against super-linear behavior on
// adversarial 1 MiB inputs. Each case takes milliseconds when linear; the
// generous bound only trips on a quadratic regression (which took > 1 minute).
func TestParsePathologicalInputs(t *testing.T) {
	const size = MaxFileSize
	cases := []struct{ name, input string }{
		{"unclosed nested braces", "K=" + strings.Repeat("${A:-", size/5-1)},
		{"unclosed nested braces double", `K="` + strings.Repeat("${A:-", size/5-1) + `"`},
		{"many unterminated double quotes", strings.Repeat("K=\"\\\n", size/5)},
		{"many unterminated single quotes", strings.Repeat("K='\n", size/4)},
		{"many unterminated backticks", strings.Repeat("K=`\\\n", size/5)},
		{"many references", "K=" + strings.Repeat("$A", size/2-1)},
		{"many directives", strings.Repeat("# envguard-ignore EG001\n", size/24-1) + "K=1"},
		{"long key without equals", strings.Repeat("A", size)},
		{"many invalid escapes", `K="` + strings.Repeat(`\q`, size/2-2) + `"`},
		{"many whitespace runs", "K=" + strings.Repeat(" a", size/2-1)},
		{"single huge line comment", "K=a #" + strings.Repeat("x", size-5)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.input) > MaxFileSize {
				t.Fatalf("input is %d bytes, larger than MaxFileSize", len(tc.input))
			}
			start := time.Now()
			mustParse(t, tc.input)
			if d := time.Since(start); d > 10*time.Second {
				t.Errorf("parsing took %v; expected linear time", d)
			}
		})
	}
}
