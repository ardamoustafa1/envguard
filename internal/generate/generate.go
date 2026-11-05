// Package generate produces the files written by `envguard init`: a starter
// .envguard.yaml and, optionally, a .env.example. It is pure: it reads the
// already-built Project and returns bytes; the CLI does the writing.
//
// It never copies values. Template values are only inspected to infer
// types; real env files are not consulted at all. Generated .env.example
// files contain empty placeholders only.
package generate

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/mask"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/scanner"
	"github.com/ardamoustafa1/envguard/internal/schema"
)

// Result holds the generated files.
type Result struct {
	Config []byte
	// Example is nil unless requested.
	Example []byte

	Variables int // in the config
	Required  int
	Sensitive int
	// ExampleVariables and ExampleGroups describe Example.
	ExampleVariables int
	ExampleGroups    int
}

// Options configures Generate.
type Options struct {
	// Example also generates a .env.example from code usages.
	Example bool
	// App limits generation to one app root ("" or "." = whole repository).
	App string
}

// variable collects what is known about one name.
type variable struct {
	name       string
	samples    []string // template values (for type inference only)
	declaredAt []string // template locations
	usedAt     []string // code locations
	refAt      []string // Compose and workflow locations
	readNoDflt bool     // some code read has no default
	readByCode bool
}

var validName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// Generate builds the files from p.
func Generate(p *project.Project, opts Options) *Result {
	vars := collect(p, opts.App)
	names := make([]string, 0, len(vars))
	for n := range vars {
		names = append(names, n)
	}
	slices.Sort(names)

	res := &Result{}
	res.Config = renderConfig(vars, names, res)
	if opts.Example {
		res.Example = renderExample(vars, names, res)
	}
	return res
}

func inApp(app, root string) bool {
	return app == "" || app == "." || root == app || strings.HasPrefix(root, app+"/")
}

func collect(p *project.Project, app string) map[string]*variable {
	vars := map[string]*variable{}
	get := func(name string) *variable {
		if !validName.MatchString(name) || scanner.IsWellKnown(name, false) {
			return nil
		}
		v := vars[name]
		if v == nil {
			v = &variable{name: name}
			vars[name] = v
		}
		return v
	}
	for _, f := range p.EnvFiles {
		if !f.IsTemplate() || !inApp(app, f.AppRoot) {
			continue
		}
		for _, e := range f.Entries() {
			if v := get(e.Key); v != nil {
				v.samples = append(v.samples, e.Value)
				v.declaredAt = append(v.declaredAt, fmt.Sprintf("%s:%d", f.Path, e.KeyPos.Line))
			}
		}
	}
	for _, u := range p.CodeUsages {
		if !u.Certain() || !inApp(app, u.AppRoot) {
			continue
		}
		if v := get(u.Variable); v != nil {
			v.readByCode = true
			v.readNoDflt = v.readNoDflt || !u.HasDefault
			v.usedAt = append(v.usedAt, fmt.Sprintf("%s:%d", u.File, u.Line))
		}
	}
	for _, r := range p.ComposeRefs {
		if app == "" || app == "." || strings.HasPrefix(r.File, app+"/") {
			if v := get(r.Variable); v != nil {
				v.refAt = append(v.refAt, fmt.Sprintf("%s:%d", r.File, r.Line))
			}
		}
	}
	for _, r := range p.WorkflowRefs {
		if app == "" || app == "." {
			if v := get(r.Variable); v != nil {
				v.refAt = append(v.refAt, fmt.Sprintf("%s:%d", r.File, r.Line))
			}
		}
	}
	return vars
}

// InferType guesses a schema type from the name and template sample values:
// *_PORT/PORT → port, *_URL/*_URI → url, *_EMAIL → email, *_HOST/*_HOSTNAME →
// hostname; otherwise from samples: true/false/yes/no/on/off → bool, digits
// → int, decimals → float, http(s)://… → url. A name-based guess is dropped
// if a real sample contradicts it. "" means string.
func InferType(name string, samples []string) string {
	n := strings.ToUpper(name)
	var guess string
	switch {
	case n == "PORT" || strings.HasSuffix(n, "_PORT"):
		guess = schema.TypePort
	case strings.HasSuffix(n, "_URL") || strings.HasSuffix(n, "_URI") || n == "URL":
		guess = schema.TypeURL
	case strings.HasSuffix(n, "_EMAIL") || n == "EMAIL":
		guess = schema.TypeEmail
	case strings.HasSuffix(n, "_HOST") || strings.HasSuffix(n, "_HOSTNAME") || n == "HOST":
		guess = schema.TypeHostname
	}
	var nonEmpty []string
	for _, s := range samples {
		if s = strings.TrimSpace(s); s != "" {
			nonEmpty = append(nonEmpty, s)
		}
	}
	if guess != "" {
		// Only a real sample may contradict the name; placeholders such as
		// <your-url> say nothing about the type.
		for _, s := range nonEmpty {
			if mask.IsPlaceholder(name, s) {
				continue
			}
			if _, ok := (schema.Spec{Type: guess}).Check(s); !ok {
				return "" // the template disagrees: stay with string
			}
		}
		return guess
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	// Placeholders like "changeme" match no type, so every non-empty sample
	// can serve as evidence here.
	for _, t := range []string{schema.TypeBool, schema.TypeInt, schema.TypeFloat} {
		if allMatch(nonEmpty, t) {
			return t
		}
	}
	if allMatch(nonEmpty, schema.TypeURL) && strings.Contains(nonEmpty[0], "://") {
		return schema.TypeURL
	}
	return ""
}

var boolWords = []string{"true", "false", "yes", "no", "on", "off"}

func allMatch(samples []string, typ string) bool {
	for _, s := range samples {
		if typ == schema.TypeBool && !slices.Contains(boolWords, strings.ToLower(s)) {
			return false // 1/0 are ints here, not bools
		}
		if _, ok := (schema.Spec{Type: typ}).Check(s); !ok {
			return false
		}
	}
	return true
}

func renderConfig(vars map[string]*variable, names []string, res *Result) []byte {
	var b strings.Builder
	b.WriteString("# Generated by `envguard init`. Review every entry: types and required flags\n")
	b.WriteString("# are inferred from your templates and code, not verified.\n")
	fmt.Fprintf(&b, "# yaml-language-server: $schema=%s\n", config.SchemaID)
	b.WriteString("version: 1\n")
	if len(names) == 0 {
		b.WriteString("\n# No variables found yet. Add them under `variables:`; see\n# https://github.com/ardamoustafa1/envguard#configuration\n")
		return []byte(b.String())
	}
	b.WriteString("\nvariables:\n")
	for _, n := range names {
		v := vars[n]
		res.Variables++
		fmt.Fprintf(&b, "  %s:\n", n)
		fmt.Fprintf(&b, "    # TODO: describe what %s is for (description: ...)\n", n)
		if where := sources(v); where != "" {
			fmt.Fprintf(&b, "    # %s\n", where)
		}
		typ := InferType(n, v.samples)
		if typ != "" {
			fmt.Fprintf(&b, "    type: %s\n", typ)
		}
		if v.readNoDflt {
			res.Required++
			b.WriteString("    required: true # read in code without a default\n")
		}
		if mask.IsSensitiveName(n) {
			res.Sensitive++
			b.WriteString("    sensitive: true\n")
		}
		if typ == "" && !v.readNoDflt && !mask.IsSensitiveName(n) {
			b.WriteString("    type: string\n") // keep every entry a valid mapping
		}
	}
	return []byte(b.String())
}

const maxSources = 3

func sources(v *variable) string {
	var parts []string
	if len(v.usedAt) > 0 {
		parts = append(parts, "read in "+list(v.usedAt))
	}
	if len(v.declaredAt) > 0 {
		parts = append(parts, "declared in "+list(v.declaredAt))
	}
	if len(v.refAt) > 0 {
		parts = append(parts, "referenced in "+list(v.refAt))
	}
	// A filename can contain a newline. Keep every location on the comment
	// line so it cannot become generated YAML content.
	return mask.Printable(strings.ReplaceAll(strings.Join(parts, "; "), "\n", `\n`))
}

func list(items []string) string {
	items = slices.Compact(slices.Clone(items))
	if len(items) <= maxSources {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:maxSources], ", ") + fmt.Sprintf(" and %d more", len(items)-maxSources)
}

// renderExample lists every variable read by code, grouped by the prefix
// before the first underscore, with empty values only.
func renderExample(vars map[string]*variable, names []string, res *Result) []byte {
	groups := map[string][]string{}
	var order []string
	for _, n := range names {
		v := vars[n]
		if !v.readByCode {
			continue
		}
		prefix, _, found := strings.Cut(n, "_")
		if !found {
			prefix = "GENERAL"
		}
		if _, ok := groups[prefix]; !ok {
			order = append(order, prefix)
		}
		groups[prefix] = append(groups[prefix], n)
	}
	slices.Sort(order)

	var b strings.Builder
	b.WriteString("# Generated by `envguard init --example`: every variable the code reads.\n")
	b.WriteString("# Copy to .env and fill in real values there. Never put real values in this file.\n")
	for _, g := range order {
		fmt.Fprintf(&b, "\n# %s\n", g)
		for _, n := range groups[g] {
			res.ExampleVariables++
			if vars[n].readNoDflt {
				fmt.Fprintf(&b, "%s=\n", n)
			} else {
				fmt.Fprintf(&b, "%s= # optional: the code has a default\n", n)
			}
		}
	}
	res.ExampleGroups = len(order)
	return []byte(b.String())
}
