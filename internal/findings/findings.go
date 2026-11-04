package findings

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Severity of a finding. The zero value is invalid. Ordering: Info < Warning < Error.
type Severity int

// Severities.
const (
	Info Severity = iota + 1
	Warning
	Error
)

// String returns "info", "warning" or "error".
func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// ParseSeverity parses "error", "warning" or "info" (case-insensitive).
func ParseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info":
		return Info, nil
	case "warning":
		return Warning, nil
	case "error":
		return Error, nil
	default:
		return 0, fmt.Errorf("invalid severity %q: want error, warning or info", s)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (s Severity) MarshalText() ([]byte, error) {
	if s < Info || s > Error {
		return nil, fmt.Errorf("invalid severity %d", int(s))
	}
	return []byte(s.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (s *Severity) UnmarshalText(b []byte) error {
	v, err := ParseSeverity(string(b))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// AtLeast reports whether s is at least as severe as min.
func (s Severity) AtLeast(minimum Severity) bool { return s >= minimum }

// Location is a position in a file. Line and Column are 1-based; 0 means
// "not applicable" (e.g. a whole-file finding).
type Location struct {
	File   string
	Line   int
	Column int
}

func (l Location) String() string {
	switch {
	case l.Line <= 0:
		return l.File
	case l.Column <= 0:
		return fmt.Sprintf("%s:%d", l.File, l.Line)
	default:
		return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column)
	}
}

// Finding is one problem reported by a rule. Message, Why and Fix never
// contain values; Evidence may only be produced by internal/mask.
type Finding struct {
	RuleID   string
	RuleName string
	Severity Severity
	Message  string
	Variable string
	File     string
	Line     int
	Column   int
	Evidence string
	Why      string
	Fix      string
	Related  []Location
	DocsURL  string
}

// Location returns the primary location of the finding.
func (f *Finding) Location() Location { return Location{File: f.File, Line: f.Line, Column: f.Column} }

// Compare orders findings by file, line, column, rule ID, then variable and
// message as tie-breakers.
func Compare(a, b *Finding) int {
	return cmp.Or(
		strings.Compare(a.File, b.File),
		cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Column, b.Column),
		strings.Compare(a.RuleID, b.RuleID),
		strings.Compare(a.Variable, b.Variable),
		strings.Compare(a.Message, b.Message),
	)
}

// Sort sorts findings deterministically (see Compare).
func Sort(fs []Finding) {
	slices.SortStableFunc(fs, func(a, b Finding) int { return Compare(&a, &b) })
}

// Dedupe sorts fs and removes findings with the same rule, location,
// variable and message, keeping the first.
func Dedupe(fs []Finding) []Finding {
	Sort(fs)
	return slices.CompactFunc(fs, func(a, b Finding) bool {
		return a.RuleID == b.RuleID && a.File == b.File && a.Line == b.Line &&
			a.Column == b.Column && a.Variable == b.Variable && a.Message == b.Message
	})
}

// Counts tallies findings by severity.
type Counts struct {
	Error, Warning, Info int
}

// Count returns the number of findings per severity.
func Count(fs []Finding) Counts {
	var c Counts
	for i := range fs {
		switch fs[i].Severity {
		case Error:
			c.Error++
		case Warning:
			c.Warning++
		case Info:
			c.Info++
		}
	}
	return c
}

// AnyAtLeast reports whether any finding is at least minimum severe.
func AnyAtLeast(fs []Finding, minimum Severity) bool {
	for i := range fs {
		if fs[i].Severity.AtLeast(minimum) {
			return true
		}
	}
	return false
}
