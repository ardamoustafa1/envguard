package rules

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
)

// Rule is one check. Check must be a pure function of the project: no I/O,
// no globals, no time or randomness. Findings it returns must have Message,
// Why and Fix set; RuleID, RuleName, DocsURL and (if zero) Severity are
// filled in by the engine.
type Rule interface {
	ID() string
	Name() string
	DefaultSeverity() findings.Severity
	Check(p *project.Project) []findings.Finding
}

// DocsBaseURL is where rule documentation is published.
const DocsBaseURL = "https://github.com/ardamoustafa1/envguard/blob/main/docs/rules/"

// DocsURL returns the documentation URL of a rule.
func DocsURL(r Rule) string { return DocsBaseURL + r.ID() + "-" + r.Name() + ".md" }

// registry is the only global mutable state allowed in EnvGuard. It is
// written only from init functions (Register) and read afterwards.
var registry = map[string]Rule{}

var ruleIDRE = regexp.MustCompile(`^EG\d{3}$`)

// Register adds a rule. It panics on an invalid or duplicate ID; call it
// only from init.
func Register(r Rule) {
	id := r.ID()
	if !ruleIDRE.MatchString(id) {
		panic("rules: invalid rule ID " + id)
	}
	if _, dup := registry[id]; dup {
		panic("rules: duplicate rule ID " + id)
	}
	registry[id] = r
}

// All returns every registered rule, sorted by ID.
func All() []Rule {
	out := make([]Rule, 0, len(registry))
	for _, r := range registry {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b Rule) int { return strings.Compare(a.ID(), b.ID()) })
	return out
}

// Lookup returns the rule with the given ID.
func Lookup(id string) (Rule, bool) {
	r, ok := registry[strings.ToUpper(id)]
	return r, ok
}

// InternalError reports a bug in a rule: a panic or an invalid finding. It
// never contains env values: panic values are only included when they are
// runtime errors (index out of range, nil dereference, ...).
type InternalError struct {
	RuleID   string
	RuleName string
	Reason   string
	Stack    []byte
}

func (e *InternalError) Error() string {
	return fmt.Sprintf("internal error in rule %s (%s): %s; please report this bug", e.RuleID, e.RuleName, e.Reason)
}

// Result is the outcome of Run.
type Result struct {
	// Findings are sorted, deduplicated and filtered by suppressions.
	Findings []findings.Finding
	// Suppressed counts findings hidden by config ignores or inline directives.
	Suppressed int
	// Rules lists the IDs of rules that ran.
	Rules []string
}

// Run executes rules concurrently against p and applies the project's
// severity overrides and suppressions. Output order does not depend on
// scheduling. Any rule panic or invalid finding makes Run return an
// *InternalError (joined if several rules fail).
func Run(p *project.Project, rules []Rule) (*Result, error) {
	sup := Suppression(p)
	var active []Rule
	for _, r := range rules {
		if !sup.RuleOff(r.ID()) {
			active = append(active, r)
		}
	}

	results := make([][]findings.Finding, len(active))
	errs := make([]error, len(active))
	var wg sync.WaitGroup
	for i, r := range active {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = runOne(r, p)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	var all []findings.Finding
	res := &Result{}
	for i, r := range active {
		all = append(all, results[i]...)
		res.Rules = append(res.Rules, r.ID())
	}
	applied := sup.Apply(all)
	res.Findings = applied.Findings
	res.Suppressed = applied.Suppressed
	return res, nil
}

func runOne(r Rule, p *project.Project) (out []findings.Finding, err error) {
	defer func() {
		if v := recover(); v != nil {
			reason := fmt.Sprintf("panic of type %T", v)
			if re, ok := v.(runtime.Error); ok {
				reason = "panic: " + re.Error()
			}
			out, err = nil, &InternalError{RuleID: r.ID(), RuleName: r.Name(), Reason: reason, Stack: debug.Stack()}
		}
	}()
	out = r.Check(p)
	for i := range out {
		f := &out[i]
		if f.Message == "" || f.Why == "" || f.Fix == "" {
			return nil, &InternalError{RuleID: r.ID(), RuleName: r.Name(),
				Reason: "finding without Message, Why or Fix (every finding must explain why it matters and how to fix it)"}
		}
		f.RuleID = r.ID()
		f.RuleName = r.Name()
		f.DocsURL = DocsURL(r)
		if f.Severity == 0 {
			f.Severity = r.DefaultSeverity()
		}
	}
	return out, nil
}

// Suppression builds the suppression settings of a project: severity
// overrides and ignores from the config, inline directives from env files.
func Suppression(p *project.Project) *findings.Suppression {
	s := &findings.Suppression{
		Inline: func(ruleID string, loc findings.Location) bool {
			return p.IsIgnored(ruleID, loc.File, loc.Line)
		},
	}
	if p.Settings != nil {
		s.Overrides = p.Settings.Rules
		for _, ig := range p.Settings.Ignore {
			s.Ignores = append(s.Ignores, findings.Ignore{Rule: ig.Rule, Path: ig.Path, Variable: ig.Variable})
		}
	}
	return s
}
