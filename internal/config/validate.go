package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/schema"
)

// Value sets shared by validation and the JSON Schema.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
	SeverityOff     = "off"
	FailOnNever     = "never"

	ProfileProductionSafety = "production-safety"
)

// enumValues returns the named value set referenced by `enum` struct tags.
func enumValues(name string) []string {
	switch name {
	case "severity":
		return []string{SeverityError, SeverityWarning, SeverityInfo}
	case "fail-on":
		return []string{SeverityError, SeverityWarning, SeverityInfo, FailOnNever}
	case "rule-severity":
		return []string{SeverityError, SeverityWarning, SeverityInfo, SeverityOff}
	case "profile":
		return []string{ProfileProductionSafety}
	case "variable-type":
		return schema.Types()
	case "language":
		langs := discovery.Languages()
		out := make([]string, len(langs))
		for i, l := range langs {
			out[i] = string(l)
		}
		return out
	default:
		panic("config: unknown enum " + name) // programming error, caught by tests
	}
}

var (
	ruleIDRE  = regexp.MustCompile(`^EG\d{3}$`)
	envNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	varNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	// varGlobRE allows '*' wildcards in variable names (ignore lists).
	varGlobRE = regexp.MustCompile(`^[A-Za-z_*][A-Za-z0-9_.*-]*$`)
)

// at returns the node for path (the value), falling back to its key node.
func (d *decoder) at(path string) *yamlNode {
	if n, ok := d.nodes[path]; ok {
		return n
	}
	if n, ok := d.keys[path]; ok {
		return n
	}
	return nil
}

func (d *decoder) keyAt(path string) *yamlNode {
	if n, ok := d.keys[path]; ok {
		return n
	}
	return d.at(path)
}

func (d *decoder) validate(f *File) {
	if _, ok := d.nodes["version"]; !ok {
		d.addf(&yamlNode{Line: 1, Column: 1}, `missing required key "version"; add "version: 1"`)
	} else if f.Version != 1 {
		d.addf(d.at("version"), "unsupported version; only version 1 is supported")
	}

	if f.FailOn != "" {
		d.oneOf(f.FailOn, "fail_on", "fail-on")
	}
	d.globs(f.Include, "include")
	d.globs(f.Exclude, "exclude")

	for _, name := range sortedKeys(f.Environments) {
		env := f.Environments[name]
		p := joinPath("environments", name)
		if !envNameRE.MatchString(name) {
			d.addf(d.keyAt(p), "environment name %q must be lower-case letters, digits, '-' or '_'", name)
		}
		if len(env.Files) == 0 && len(env.Templates) == 0 {
			d.addf(d.keyAt(p), "environment %q must list at least one of \"files\" or \"templates\"", name)
		}
		d.globs(env.Files, p+".files")
		d.globs(env.Templates, p+".templates")
	}

	for _, id := range sortedKeys(f.Rules) {
		p := joinPath("rules", id)
		if !ruleIDRE.MatchString(id) {
			d.addf(d.keyAt(p), "rule key %q is not a rule ID; use IDs like EG004", id)
			continue
		}
		d.oneOf(f.Rules[id], p, "rule-severity")
	}

	for i, ig := range f.Ignore {
		p := fmt.Sprintf("ignore[%d]", i)
		if ig.Path == "" && ig.Variable == "" {
			d.addf(d.at(p), `ignore entry must set "path" and/or "variable" (use rules: {EGxxx: off} to disable a rule everywhere)`)
		}
		if ig.Rule != "" && !ruleIDRE.MatchString(ig.Rule) {
			d.addf(d.at(p+".rule"), "%q is not a rule ID; use IDs like EG004", ig.Rule)
		}
		if ig.Path != "" {
			d.glob(ig.Path, p+".path")
		}
		if ig.Variable != "" && !varGlobRE.MatchString(ig.Variable) {
			d.addf(d.at(p+".variable"), "invalid variable name or pattern")
		}
	}

	for i, lang := range f.Scanner.Languages {
		d.oneOf(lang, fmt.Sprintf("scanner.languages[%d]", i), "language")
	}
	for i, v := range f.Scanner.IgnoreVariables {
		if !varGlobRE.MatchString(v) {
			d.addf(d.at(fmt.Sprintf("scanner.ignore_variables[%d]", i)), "invalid variable name or pattern")
		}
	}

	for i, prof := range f.Profiles {
		d.oneOf(prof, fmt.Sprintf("profiles[%d]", i), "profile")
	}

	for _, name := range sortedKeys(f.Variables) {
		d.variable(name, f.Variables[name])
	}
}

// variable validates one schema entry: name, type, the Spec constraints
// (via schema.ValidateSpec), environments, default and production rule.
// Messages name the offending key; values from the config are never echoed.
func (d *decoder) variable(name string, v Variable) {
	p := joinPath("variables", name)
	if !varNameRE.MatchString(name) {
		d.addf(d.keyAt(p), "variable name %q is invalid: use letters, digits, '_', '.' or '-', not starting with a digit", name)
	}
	if v.Type != "" && !slices.Contains(schema.Types(), v.Type) {
		d.oneOf(v.Type, p+".type", "variable-type")
		return // other checks depend on a known type
	}
	spec := v.Spec()
	for _, e := range schema.ValidateSpec(spec) {
		d.addf(d.at(p+"."+e.Field), "%s: %s", label(p), e.Message)
	}
	for i, e := range v.Environments {
		if !envNameRE.MatchString(e) {
			d.addf(d.at(fmt.Sprintf("%s.environments[%d]", p, i)), "invalid environment name")
		}
	}
	if len(v.Environments) > 0 && !v.Required {
		d.addf(d.at(p+".environments"), `%s: "environments" has no effect unless "required: true" is set`, label(p))
	}
	if v.Default != "" {
		if reason, ok := spec.Check(v.Default); !ok {
			d.addf(d.at(p+".default"), "%s: default %s", label(p), reason)
		}
	}
	if r := v.Production; r != nil {
		pp := p + ".production"
		for _, fv := range [][2]string{{"must_be", r.MustBe}, {"must_not_be", r.MustNotBe}} {
			field, val := fv[0], fv[1]
			if val == "" {
				continue
			}
			if reason, ok := (schema.Spec{Type: v.Type, Values: v.Values, Schemes: v.Schemes}).Check(val); !ok {
				d.addf(d.at(pp+"."+field), "%s: %s %s", label(pp), field, reason)
			}
		}
		if r.MustBe != "" && r.MustNotBe != "" && spec.Equal(r.MustBe, r.MustNotBe) {
			d.addf(d.at(pp), `%s: "must_be" and "must_not_be" are equal`, label(pp))
		}
		if r.Pattern != "" {
			if _, err := regexp.Compile(r.Pattern); err != nil {
				d.addf(d.at(pp+".pattern"), "%s: invalid regular expression (Go RE2 syntax)", label(pp))
			}
		}
		if r.MustBe == "" && r.MustNotBe == "" && r.Pattern == "" {
			d.addf(d.at(pp), `%s: set at least one of "must_be", "must_not_be" or "pattern"`, label(pp))
		}
	}
}

// oneOf checks value against a named enum and suggests the closest match.
func (d *decoder) oneOf(value, path, enum string) {
	allowed := enumValues(enum)
	if slices.Contains(allowed, value) {
		return
	}
	d.addf(d.at(path), "invalid value for %s; allowed: %s%s", label(path), strings.Join(allowed, ", "), suggestion(value, allowed))
}

func (d *decoder) globs(patterns []string, path string) {
	for i, g := range patterns {
		d.glob(g, fmt.Sprintf("%s[%d]", path, i))
	}
}

func (d *decoder) glob(g, path string) {
	switch {
	case strings.TrimSpace(g) == "":
		d.addf(d.at(path), "%s must not be empty", label(path))
	case strings.Contains(g, `\`):
		d.addf(d.at(path), "%s must use forward slashes", label(path))
	case strings.HasPrefix(g, "/") || slices.Contains(strings.Split(g, "/"), "..") || isWindowsAbs(g):
		d.addf(d.at(path), "%s must stay inside the root: no absolute paths and no \"..\" segments", label(path))
	case !doublestar.ValidatePattern(g):
		d.addf(d.at(path), "%s is not a valid glob pattern", label(path))
	}
}

// isWindowsAbs reports drive-letter paths such as C:/x.
func isWindowsAbs(g string) bool {
	return len(g) >= 2 && g[1] == ':' && (g[0] >= 'a' && g[0] <= 'z' || g[0] >= 'A' && g[0] <= 'Z')
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
