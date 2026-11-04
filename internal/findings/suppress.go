package findings

import (
	"path"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Off disables a rule in Suppression.Overrides.
const Off = "off"

// Ignore suppresses findings; every non-empty field must match.
type Ignore struct {
	// Rule is a rule ID; "" matches every rule.
	Rule string
	// Path is a doublestar glob relative to the root. A pattern without '/'
	// matches the base name at any depth.
	Path string
	// Variable is a name where '*' matches any sequence of characters.
	Variable string
}

// Suppression decides which findings are reported.
type Suppression struct {
	// Overrides maps rule IDs to a severity ("error", "warning", "info") or
	// "off". Off rules should not be run at all; findings of off rules that
	// reach Apply are dropped without being counted as suppressed.
	Overrides map[string]string
	// Ignores come from the config file.
	Ignores []Ignore
	// Inline reports whether an inline directive (# envguard-ignore,
	// # envguard-ignore-file) suppresses ruleID at loc.
	Inline func(ruleID string, loc Location) bool
}

// Result is the outcome of Apply.
type Result struct {
	Findings []Finding
	// Suppressed counts findings removed by config ignores or inline
	// directives (not by "off" overrides).
	Suppressed int
}

// RuleOff reports whether a rule is disabled.
func (s *Suppression) RuleOff(ruleID string) bool {
	return strings.EqualFold(s.Overrides[ruleID], Off)
}

// Apply applies severity overrides, then suppressions, then dedupes and
// sorts. A finding is inline-suppressed when a directive matches its primary
// location or any Related location (so "# envguard-ignore EG001" above a
// template line also silences "missing" findings that point to it).
func (s *Suppression) Apply(in []Finding) Result {
	var res Result
	for i := range in {
		f := in[i]
		if s.RuleOff(f.RuleID) {
			continue
		}
		if o := s.Overrides[f.RuleID]; o != "" {
			if sev, err := ParseSeverity(o); err == nil {
				f.Severity = sev
			}
		}
		if s.suppressed(&f) {
			res.Suppressed++
			continue
		}
		res.Findings = append(res.Findings, f)
	}
	res.Findings = Dedupe(res.Findings)
	return res
}

func (s *Suppression) suppressed(f *Finding) bool {
	for _, ig := range s.Ignores {
		if ig.matches(f) {
			return true
		}
	}
	if s.Inline == nil {
		return false
	}
	if s.Inline(f.RuleID, f.Location()) {
		return true
	}
	for _, r := range f.Related {
		if s.Inline(f.RuleID, r) {
			return true
		}
	}
	return false
}

func (ig Ignore) matches(f *Finding) bool {
	if ig.Rule != "" && ig.Rule != f.RuleID {
		return false
	}
	if ig.Path != "" && !MatchPath(ig.Path, f.File) {
		return false
	}
	if ig.Variable != "" && (f.Variable == "" || !MatchName(ig.Variable, f.Variable)) {
		return false
	}
	return true
}

// MatchPath matches a slash-separated relative path against a doublestar
// glob. A pattern without '/' matches the base name at any depth.
func MatchPath(pattern, p string) bool {
	target := p
	if !strings.Contains(pattern, "/") {
		target = path.Base(p)
	}
	ok, err := doublestar.Match(pattern, target)
	return err == nil && ok
}

// MatchName matches a variable name against a pattern where '*' matches any
// sequence of characters. Matching is case-sensitive.
func MatchName(pattern, name string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == name
	}
	parts := strings.Split(pattern, "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	re, err := regexp.Compile("^" + strings.Join(parts, ".*") + "$")
	return err == nil && re.MatchString(name)
}
