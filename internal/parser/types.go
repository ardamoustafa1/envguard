package parser

import "slices"

// MaxFileSize is the largest env file Parse will read. Larger inputs produce a
// single ProblemFileTooLarge and no entries.
const MaxFileSize = 1 << 20 // 1 MiB

// Quote describes how a value was quoted in the source.
type Quote string

// Quote styles.
const (
	QuoteNone     Quote = "none"
	QuoteSingle   Quote = "single"
	QuoteDouble   Quote = "double"
	QuoteBacktick Quote = "backtick"
)

// Severity of a parse problem.
type Severity string

// Problem severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// ProblemKind identifies the class of a parse problem.
type ProblemKind string

// Problem kinds.
const (
	ProblemFileTooLarge       ProblemKind = "file-too-large"
	ProblemInvalidKey         ProblemKind = "invalid-key"
	ProblemEmptyKey           ProblemKind = "empty-key"
	ProblemMissingEquals      ProblemKind = "missing-equals"
	ProblemUnterminatedQuote  ProblemKind = "unterminated-quote"
	ProblemUnquotedWhitespace ProblemKind = "unquoted-whitespace"
	ProblemInvalidEscape      ProblemKind = "invalid-escape"
	ProblemTrailingCharacters ProblemKind = "trailing-characters"
	ProblemInvalidDirective   ProblemKind = "invalid-directive"
)

// Pos is a position in the input. Line and Column are 1-based; Column counts
// Unicode code points (a tab is one column). Offset is the 0-based byte offset
// in the original input, including any UTF-8 BOM.
type Pos struct {
	Line   int
	Column int
	Offset int
}

// Reference is a variable reference ($VAR or ${VAR...}) found in an unquoted
// or double-quoted value.
type Reference struct {
	Name string
	// Pos is the position of the '$'.
	Pos Pos
	// Braced is true for the ${VAR} form.
	Braced bool
	// HasDefault is true for ${VAR:-x}, ${VAR-x}, ${VAR:=x} and ${VAR=x}.
	HasDefault bool
}

// Entry is one KEY=VALUE assignment.
//
// Value and RawValue may contain secrets. They must never be written to any
// output except through internal/mask.
type Entry struct {
	Key string
	// Value is the fully unescaped value. Newlines inside multi-line values
	// are normalized to "\n". Variable references are not expanded.
	Value string
	// RawValue is the value exactly as written, including quotes, excluding
	// any inline comment and surrounding whitespace.
	RawValue string
	Quote    Quote
	KeyPos   Pos
	// ValuePos is the start of RawValue (the opening quote for quoted values),
	// or the position right after '=' for empty unquoted values.
	ValuePos  Pos
	HasExport bool
	// InlineComment is the text after '#' on the value's last line, trimmed.
	InlineComment string
	References    []Reference
	// IgnoreRules lists rule IDs suppressed for this entry by envguard-ignore
	// directives (sorted, unique).
	IgnoreRules []string
}

// Problem is a malformed or suspicious construct. Message and Hint never
// contain text taken from the input; Key is set only when the key is valid.
type Problem struct {
	Kind     ProblemKind
	Severity Severity
	Pos      Pos
	Key      string
	Message  string
	Hint     string
}

// DirectiveScope says what an envguard-ignore directive applies to.
type DirectiveScope string

// Directive scopes.
const (
	ScopeEntry DirectiveScope = "entry"
	ScopeFile  DirectiveScope = "file"
)

// Directive is a parsed envguard-ignore or envguard-ignore-file comment.
type Directive struct {
	// Pos is the position of the '#'.
	Pos   Pos
	Scope DirectiveScope
	Rules []string
	// TargetLine is the line of the assignment an entry-scoped directive
	// applies to (also set when that line is malformed, so EG007 can be
	// suppressed). 0 for file scope or when no assignment follows.
	TargetLine int
}

// File is the result of parsing one env file.
type File struct {
	Filename   string
	Entries    []Entry
	Problems   []Problem
	Directives []Directive
	// FileIgnoreRules lists rule IDs suppressed for the whole file (sorted,
	// unique).
	FileIgnoreRules []string
}

// IsIgnored reports whether ruleID is suppressed at line, either for the whole
// file or by an entry-scoped directive targeting that line.
func (f *File) IsIgnored(ruleID string, line int) bool {
	if slices.Contains(f.FileIgnoreRules, ruleID) {
		return true
	}
	if line <= 0 {
		return false
	}
	for _, d := range f.Directives {
		if d.Scope == ScopeEntry && d.TargetLine == line && slices.Contains(d.Rules, ruleID) {
			return true
		}
	}
	return false
}
