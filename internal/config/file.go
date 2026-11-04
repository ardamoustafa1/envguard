package config

import (
	"slices"

	"github.com/ardamoustafa1/envguard/internal/schema"
)

// File is the on-disk shape of .envguard.yaml.
//
// Struct tags drive three things at once, so they must stay accurate:
//   - yaml:     the key name (strict decoding rejects anything else)
//   - desc:     the description in docs/envguard.schema.json
//   - enum:     a named value set (see enumValues) for strings, list items or map values
//   - keys:     a regular expression map keys must match (schema propertyNames)
//   - required: "true" marks a required key
//   - const:    an integer the value must equal
//
// TestJSONSchemaInSync regenerates the schema from these structs.
type File struct {
	Version int    `yaml:"version" desc:"Configuration format version. Must be 1." required:"true" const:"1"`
	FailOn  string `yaml:"fail_on" desc:"Lowest severity that makes envguard exit with code 1 (never: always exit 0 unless envguard itself fails). Overridden by --fail-on. Default: error." enum:"fail-on"`

	Include []string `yaml:"include" desc:"Glob patterns (relative to the root, ** supported). When set, only matching files are analyzed. Patterns without a slash match file names anywhere."`
	Exclude []string `yaml:"exclude" desc:"Glob patterns of files or directories to skip, in addition to .gitignore and the built-in excludes (node_modules, vendor, dist, ...). Patterns without a slash match names anywhere."`

	Environments map[string]Environment `yaml:"environments" desc:"Map environment names to env files and templates whose names envguard cannot infer (e.g. deploy/prod.env). Overrides the built-in .env.<name> inference for matching files." keys:"^[a-z0-9][a-z0-9_-]*$"`

	Rules map[string]string `yaml:"rules" desc:"Per-rule severity overrides, keyed by rule ID. Use off to disable a rule." keys:"^EG[0-9]{3}$" enum:"rule-severity"`

	Ignore []IgnoreEntry `yaml:"ignore" desc:"Suppress findings matching a rule, a path glob and/or a variable name."`

	Scanner Scanner `yaml:"scanner" desc:"Source code scanner settings."`

	Profiles []string `yaml:"profiles" desc:"Opt-in rule packs." enum:"profile"`

	Variables map[string]Variable `yaml:"variables" desc:"Schema of environment variables, keyed by variable name." keys:"^[A-Za-z_][A-Za-z0-9_.-]*$"`

	// Positions maps variable names to the position of their key in the
	// file (filled by Parse; not part of the YAML shape).
	Positions map[string]Position `yaml:"-"`
}

// Position is a 1-based line and column in the config file.
type Position struct {
	Line, Column int
}

// Environment lists files belonging to one named environment.
type Environment struct {
	Files     []string `yaml:"files" desc:"Glob patterns of real env files for this environment."`
	Templates []string `yaml:"templates" desc:"Glob patterns of env templates for this environment."`
}

// IgnoreEntry suppresses findings. All set fields must match.
type IgnoreEntry struct {
	Rule     string `yaml:"rule" desc:"Rule ID to suppress, e.g. EG004. Omit to suppress every rule." pattern:"^EG[0-9]{3}$"`
	Path     string `yaml:"path" desc:"Glob of files the suppression applies to (relative to the root, ** supported)."`
	Variable string `yaml:"variable" desc:"Variable name the suppression applies to. * matches any sequence of characters."`
	Reason   string `yaml:"reason" desc:"Why this is suppressed (for reviewers; not used by envguard)."`
}

// Scanner configures source code scanning.
type Scanner struct {
	Languages       []string `yaml:"languages" desc:"Languages to scan. Default: all supported languages." enum:"language"`
	IgnoreVariables []string `yaml:"ignore_variables" desc:"Variable names the scanner should ignore, e.g. variables provided by the platform. * matches any sequence of characters."`
}

// Variable describes one environment variable. Value semantics are
// implemented by internal/schema (see Spec).
type Variable struct {
	Description  string          `yaml:"description" desc:"What the variable is for."`
	Type         string          `yaml:"type" desc:"Value type. Default: string. bool accepts true/false, 1/0, yes/no, on/off (case-insensitive); duration uses Go syntax (300ms, 1h30m)." enum:"variable-type"`
	Required     bool            `yaml:"required" desc:"The variable must be set (in the listed environments, or everywhere). Default: false."`
	Environments []string        `yaml:"environments" desc:"Environments in which required applies. Default: all environments."`
	Sensitive    bool            `yaml:"sensitive" desc:"Always mask the value, even if the name looks harmless."`
	Default      string          `yaml:"default" desc:"Documented default value. Must be valid for the type."`
	Values       []string        `yaml:"values" desc:"Allowed values (required for type enum, only allowed there)."`
	Schemes      []string        `yaml:"schemes" desc:"Allowed URL schemes (type url only), e.g. [postgres, postgresql]."`
	Pattern      string          `yaml:"pattern" desc:"Regular expression (Go RE2 syntax) the value must match; it is searched, so anchor with ^ and $ for a full match."`
	Min          *float64        `yaml:"min" desc:"Minimum numeric value (int, float, port)."`
	Max          *float64        `yaml:"max" desc:"Maximum numeric value (int, float, port)."`
	MinLength    *int            `yaml:"min_length" desc:"Minimum value length in characters."`
	MaxLength    *int            `yaml:"max_length" desc:"Maximum value length in characters."`
	Production   *ProductionRule `yaml:"production" desc:"Extra constraints in the production environment (rule EG015)."`
}

// ProductionRule constrains a variable in the production environment.
type ProductionRule struct {
	MustBe    string `yaml:"must_be" desc:"The value must equal this in production (compared by type: for bool, off equals false)."`
	MustNotBe string `yaml:"must_not_be" desc:"The value must not equal this in production (compared by type)."`
	Pattern   string `yaml:"pattern" desc:"Regular expression the value must match in production."`
}

// Spec returns the value constraints of v for internal/schema.
func (v Variable) Spec() schema.Spec {
	return schema.Spec{Type: v.Type, Values: v.Values, Schemes: v.Schemes, Pattern: v.Pattern,
		Min: v.Min, Max: v.Max, MinLength: v.MinLength, MaxLength: v.MaxLength}
}

// RequiredIn reports whether the variable is required in env.
func (v Variable) RequiredIn(env string) bool {
	return v.Required && (len(v.Environments) == 0 || slices.Contains(v.Environments, env))
}
