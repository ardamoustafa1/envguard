package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

const fullConfig = `version: 1
fail_on: warning
include: ["apps/**"]
exclude:
  - "**/fixtures/**"
  - legacy
environments:
  production:
    files: [deploy/prod.env]
    templates: [deploy/prod.env.example]
rules:
  EG004: off
  EG002: error
ignore:
  - rule: EG003
    path: "scripts/**"
    reason: scripts read CI-provided vars
  - variable: "NEXT_PUBLIC_*"
scanner:
  languages: [go, typescript]
  ignore_variables: [HOME, PATH, "CI_*"]
profiles: [production-safety]
variables:
  DATABASE_URL:
    description: Postgres connection string
    required: true
    type: url
    schemes: [postgres, postgresql]
    sensitive: true
    environments: [staging, production]
  LOG_LEVEL:
    type: enum
    values: [debug, info, warn, error]
    default: info
  PORT: { type: port, default: "3000" }
  WORKERS: { type: int, min: 1, max: 64 }
  DEBUG:
    type: bool
    production: { must_be: false }
  API_URL: { type: url, pattern: "^https://" }
  API_KEY:
    pattern: "^[a-z0-9]{32}$"
    min_length: 32
    max_length: 32
`

func TestParseFullConfig(t *testing.T) {
	f, err := Parse([]byte(fullConfig), ".envguard.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Version != 1 || f.FailOn != "warning" {
		t.Errorf("version/fail_on = %d/%q", f.Version, f.FailOn)
	}
	if !slices.Equal(f.Exclude, []string{"**/fixtures/**", "legacy"}) {
		t.Errorf("Exclude = %v", f.Exclude)
	}
	if got := f.Environments["production"].Files; !slices.Equal(got, []string{"deploy/prod.env"}) {
		t.Errorf("Environments = %+v", f.Environments)
	}
	if f.Rules["EG004"] != "off" || f.Rules["EG002"] != "error" {
		t.Errorf("Rules = %v", f.Rules)
	}
	if len(f.Ignore) != 2 || f.Ignore[0].Rule != "EG003" || f.Ignore[1].Variable != "NEXT_PUBLIC_*" {
		t.Errorf("Ignore = %+v", f.Ignore)
	}
	if !slices.Equal(f.Scanner.Languages, []string{"go", "typescript"}) {
		t.Errorf("Scanner = %+v", f.Scanner)
	}
	if !slices.Equal(f.Profiles, []string{"production-safety"}) {
		t.Errorf("Profiles = %v", f.Profiles)
	}
	lvl := f.Variables["LOG_LEVEL"]
	if lvl.Required || lvl.Type != "enum" || lvl.Default != "info" {
		t.Errorf("LOG_LEVEL = %+v", lvl)
	}
	db := f.Variables["DATABASE_URL"]
	if !db.Required || !db.Sensitive || !db.RequiredIn("production") || db.RequiredIn("development") ||
		!slices.Equal(db.Schemes, []string{"postgres", "postgresql"}) {
		t.Errorf("DATABASE_URL = %+v", db)
	}
	if p := f.Variables["PORT"]; p.Default != "3000" || p.Type != "port" {
		t.Errorf("PORT = %+v", p)
	}
	if w := f.Variables["WORKERS"]; w.Min == nil || *w.Min != 1 || *w.Max != 64 {
		t.Errorf("WORKERS = %+v", w)
	}
	if d := f.Variables["DEBUG"]; d.Production == nil || d.Production.MustBe != "false" {
		t.Errorf("DEBUG = %+v", d)
	}
	if pos := f.Positions["WORKERS"]; pos.Line != 36 || pos.Column != 3 {
		t.Errorf("WORKERS position = %+v", pos)
	}
}

func TestParseMinimal(t *testing.T) {
	f, err := Parse([]byte("version: 1\n"), "c.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 1 || f.Rules != nil {
		t.Errorf("got %+v", f)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // substrings; each must appear in the error
	}{
		{"unknown top-level key with suggestion", "version: 1\nexlude: [x]\n",
			[]string{`c.yaml:2:1: unknown key "exlude" in the top level (did you mean "exclude"?)`}},
		{"unknown nested key", "version: 1\nscanner:\n  languges: [go]\n",
			[]string{`c.yaml:3:3: unknown key "languges" in "scanner" (did you mean "languages"?)`}},
		{"unknown key in list item", "version: 1\nignore:\n  - rule: EG001\n    pth: x\n",
			[]string{`c.yaml:4:5: unknown key "pth" in "ignore[0]" (did you mean "path"?)`}},
		{"unknown key in variable", "version: 1\nvariables:\n  A:\n    requird: true\n",
			[]string{`unknown key "requird" in "variables.A" (did you mean "required"?)`}},
		{"no suggestion when far", "version: 1\ncompletely_unrelated: 1\n",
			[]string{`unknown key "completely_unrelated" in the top level`}},
		{"prefix suggestion", "version: 1\nscanner:\n  ignore_vars: [A]\n",
			[]string{`(did you mean "ignore_variables"?)`}},
		{"several problems reported together", "version: 1\nexlude: []\ninclde: []\n",
			[]string{"2 problems", `"exlude"`, `"inclde"`}},
		{"unknown keys and bad values reported together, in file order", "rules:\n  EG004: warn\nexlude: []\n",
			[]string{"3 problems", "c.yaml:1:1: missing required key", "c.yaml:2:10: invalid value", `c.yaml:3:1: unknown key "exlude"`}},
		{"missing version", "exclude: [a]\n", []string{`c.yaml:1:1: missing required key "version"`}},
		{"wrong version", "version: 2\n", []string{"c.yaml:1:10: unsupported version"}},
		{"version as string", "version: \"1\"\n", []string{`"version" must be an integer`}},
		{"empty file", "", []string{"config file is empty"}},
		{"comment only", "# nothing\n", []string{"config file is empty"}},
		{"not a mapping", "- a\n- b\n", []string{"the top level must be a mapping, got a list"}},
		{"list expected", "version: 1\nexclude: foo\n", []string{`c.yaml:2:10: "exclude" must be a list, got a scalar value`}},
		{"mapping expected", "version: 1\nrules: [EG001]\n", []string{`"rules" must be a mapping, got a list`}},
		{"bool expected", "version: 1\nvariables:\n  A:\n    sensitive: \"yes\"\n", []string{`"variables.A.sensitive" must be true or false`}},
		{"number expected", "version: 1\nvariables:\n  A:\n    min: low\n", []string{`"variables.A.min" must be a number`}},
		{"invalid yaml", "version: 1\n  bad: [\n", []string{"invalid YAML"}},
		{"duplicate key", "version: 1\nversion: 1\n", []string{`c.yaml:2:1: duplicate key "version" in the top level (first defined at line 1)`}},
		{"duplicate map key", "version: 1\nrules:\n  EG001: off\n  EG001: info\n", []string{`c.yaml:4:3: duplicate key "EG001" in "rules"`}},
		{"bad fail_on", "version: 1\nfail_on: warn\n", []string{`invalid value for "fail_on"; allowed: error, warning, info, never (did you mean "warning"?)`}},
		{"bad rule id", "version: 1\nrules:\n  missing-variable: off\n", []string{`c.yaml:3:3: rule key "missing-variable" is not a rule ID`}},
		{"bad rule severity", "version: 1\nrules:\n  EG001: warnings\n", []string{`invalid value for "rules.EG001"`, `(did you mean "warning"?)`}},
		{"yaml false is not off", "version: 1\nrules:\n  EG001: false\n", []string{`invalid value for "rules.EG001"`}},
		{"bad glob", "version: 1\nexclude: [\"a[\"]\n", []string{`"exclude[0]" is not a valid glob pattern`}},
		{"absolute glob", "version: 1\ninclude: [/etc/**]\n", []string{`"include[0]" must stay inside the root`}},
		{"parent glob", "version: 1\nexclude: [\"../**\"]\n", []string{`"exclude[0]" must stay inside the root`}},
		{"parent segment in the middle", "version: 1\nexclude: [\"a/../../b\"]\n", []string{`"exclude[0]" must stay inside the root`}},
		{"bare parent", "version: 1\ninclude: [\"..\"]\n", []string{`"include[0]" must stay inside the root`}},
		{"windows drive glob", "version: 1\ninclude: [\"C:/Windows/**\"]\n", []string{`"include[0]" must stay inside the root`}},
		{"backslash glob", "version: 1\nexclude: ['a\\b']\n", []string{"must use forward slashes"}},
		{"empty glob", "version: 1\nexclude: [\"\"]\n", []string{`"exclude[0]" must not be empty`}},
		{"bad environment name", "version: 1\nenvironments:\n  Prod:\n    files: [a]\n", []string{`environment name "Prod" must be lower-case`}},
		{"empty environment", "version: 1\nenvironments:\n  prod: {}\n", []string{`environment "prod" must list at least one`}},
		{"ignore without scope", "version: 1\nignore:\n  - rule: EG001\n", []string{`ignore entry must set "path" and/or "variable"`}},
		{"ignore bad rule", "version: 1\nignore:\n  - rule: E1\n    path: a\n", []string{`"E1" is not a rule ID`}},
		{"bad language", "version: 1\nscanner:\n  languages: [golang]\n", []string{`invalid value for "scanner.languages[0]"`}},
		{"language suggestion", "version: 1\nscanner:\n  languages: [pyhton]\n", []string{`(did you mean "python"?)`}},
		{"bad profile", "version: 1\nprofiles: [production_safety]\n", []string{`(did you mean "production-safety"?)`}},
		{"bad variable name", "version: 1\nvariables:\n  1A: {}\n", []string{`variable name "1A" is invalid`}},
		{"bad variable type", "version: 1\nvariables:\n  A:\n    type: integer\n", []string{`invalid value for "variables.A.type"`}},
		{"enum without values", "version: 1\nvariables:\n  A:\n    type: enum\n", []string{`c.yaml:4:11: "variables.A": type enum requires a non-empty "values" list`}},
		{"bad regexp", "version: 1\nvariables:\n  A:\n    pattern: \"(\"\n", []string{`c.yaml:4:14: "variables.A": invalid regular expression`}},
		{"min greater than max", "version: 1\nvariables:\n  A:\n    min: 5\n    max: 1\n", []string{`"min" is greater than "max"`}},
		{"negative length", "version: 1\nvariables:\n  A:\n    min_length: -1\n", []string{`"min_length" must not be negative`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.input), "c.yaml")
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error does not match ErrInvalid: %v", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q\ndoes not contain %q", err.Error(), w)
				}
			}
		})
	}
}

// TestErrorsDoNotEchoValues guards principle 3 for config errors.
func TestErrorsDoNotEchoValues(t *testing.T) {
	const s = "s3cr3tSENTINEL"
	inputs := []string{
		"version: " + s + "\n",
		"version: 1\nfail_on: " + s + "\n",
		"version: 1\nrules:\n  EG001: " + s + "\n",
		"version: 1\nvariables:\n  A:\n    min: " + s + "\n",
		"version: 1\nvariables:\n  A:\n    type: " + s + "\n",
		"version: 1\nvariables:\n  A:\n    pattern: \"(" + s + "\"\n",
		"version: 1\nvariables:\n  A:\n    sensitive: " + s + "\n",
		"version: 1\nvariables:\n  A:\n    type: int\n    default: " + s + "\n",
		"version: 1\nvariables:\n  A:\n    type: bool\n    production: { must_be: " + s + " }\n",
		"version: 1\nvariables:\n  A:\n    type: enum\n    values: [a]\n    default: " + s + "\n",
		"version: 1\nscanner:\n  ignore_variables: [\"" + s + " x\"]\n",
		"version: 1\nignore:\n  - variable: \"" + s + " x\"\n",
	}
	for _, in := range inputs {
		_, err := Parse([]byte(in), "c.yaml")
		if err == nil {
			t.Errorf("%q: want error", in)
			continue
		}
		if strings.Contains(err.Error(), s) {
			t.Errorf("error leaks value: %v", err)
		}
	}
}

func TestSuggestion(t *testing.T) {
	cands := []string{"exclude", "include", "environments", "ignore", "version"}
	tests := map[string]string{
		"exlude":   ` (did you mean "exclude"?)`,
		"Exclude":  ` (did you mean "exclude"?)`,
		"ignor":    ` (did you mean "ignore"?)`,
		"env":      "",
		"environm": ` (did you mean "environments"?)`,
		"zzzzzzz":  "",
		"exclude":  "",
	}
	for in, want := range tests {
		if got := suggestion(in, cands); got != want {
			t.Errorf("suggestion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"a", "", 1}, {"", "abc", 3}, {"kitten", "sitting", 3}, {"exlude", "exclude", 1}, {"日本", "日", 1},
	}
	for _, tt := range tests {
		if got := levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestMergePrecedence(t *testing.T) {
	file := &File{Version: 1, FailOn: "warning", Exclude: []string{"from-config"}, Rules: map[string]string{"EG004": "off"}}

	tests := []struct {
		name        string
		file        *File
		flags       Flags
		wantFailOn  string
		wantExclude []string
		wantVerbose bool
	}{
		{"defaults", nil, Flags{}, "error", nil, false},
		{"config over defaults", file, Flags{}, "warning", []string{"from-config"}, false},
		{"flags over config", file, Flags{FailOn: ptr("info"), Exclude: []string{"from-flag"}, Verbose: ptr(true)}, "info", []string{"from-flag"}, true},
		{"empty flag slice still overrides", file, Flags{Exclude: []string{}}, "warning", []string{}, false},
		{"flags over defaults", nil, Flags{FailOn: ptr("warning")}, "warning", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Merge(Defaults(), tt.file, tt.flags)
			if s.FailOn != tt.wantFailOn {
				t.Errorf("FailOn = %q, want %q", s.FailOn, tt.wantFailOn)
			}
			if !slices.Equal(s.Exclude, tt.wantExclude) {
				t.Errorf("Exclude = %v, want %v", s.Exclude, tt.wantExclude)
			}
			if s.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v", s.Verbose)
			}
		})
	}

	// The result must not share memory with the config file.
	s := Merge(Defaults(), file, Flags{})
	s.Rules["EG001"] = "info"
	s.Exclude[0] = "mutated"
	if _, ok := file.Rules["EG001"]; ok || file.Exclude[0] != "from-config" {
		t.Error("Merge result aliases the config file")
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	t.Run("no config is fine", func(t *testing.T) {
		dir := t.TempDir()
		s, err := Load(Flags{Root: dir})
		if err != nil {
			t.Fatal(err)
		}
		if s.ConfigPath != "" || s.FailOn != "error" || !filepath.IsAbs(s.Root) {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("yaml in root", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".envguard.yaml", "version: 1\nfail_on: info\n")
		s, err := Load(Flags{Root: dir})
		if err != nil {
			t.Fatal(err)
		}
		if s.FailOn != "info" || !strings.HasSuffix(s.ConfigPath, "/.envguard.yaml") {
			t.Errorf("got %+v", s)
		}
	})
	t.Run("yml in root", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".envguard.yml", "version: 1\n")
		s, err := Load(Flags{Root: dir})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(s.ConfigPath, "/.envguard.yml") {
			t.Errorf("ConfigPath = %q", s.ConfigPath)
		}
	})
	t.Run("both is an error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".envguard.yaml", "version: 1\n")
		writeFile(t, dir, ".envguard.yml", "version: 1\n")
		_, err := Load(Flags{Root: dir})
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "keep only one") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("explicit config", func(t *testing.T) {
		dir := t.TempDir()
		p := writeFile(t, dir, "cfg/custom.yaml", "version: 1\nfail_on: warning\n")
		s, err := Load(Flags{Root: dir, Config: p})
		if err != nil {
			t.Fatal(err)
		}
		if s.FailOn != "warning" {
			t.Errorf("FailOn = %q", s.FailOn)
		}
	})
	t.Run("explicit config missing", func(t *testing.T) {
		_, err := Load(Flags{Root: t.TempDir(), Config: filepath.Join(t.TempDir(), "nope.yaml")})
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "not found") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("explicit config is dir", func(t *testing.T) {
		dir := t.TempDir()
		_, err := Load(Flags{Root: dir, Config: dir})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("invalid config", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".envguard.yaml", "version: 1\nexlude: []\n")
		_, err := Load(Flags{Root: dir})
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), ".envguard.yaml:2:1") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("flags beat config", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".envguard.yaml", "version: 1\nfail_on: info\n")
		s, err := Load(Flags{Root: dir, FailOn: ptr("warning")})
		if err != nil {
			t.Fatal(err)
		}
		if s.FailOn != "warning" {
			t.Errorf("FailOn = %q", s.FailOn)
		}
	})
	t.Run("missing root", func(t *testing.T) {
		if _, err := Load(Flags{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
			t.Error("want error")
		}
	})
	t.Run("root is a file", func(t *testing.T) {
		p := writeFile(t, t.TempDir(), "f", "x")
		if _, err := Load(Flags{Root: p}); err == nil {
			t.Error("want error")
		}
	})
	t.Run("oversized config", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".envguard.yaml", "version: 1\n#"+strings.Repeat("x", maxConfigSize))
		_, err := Load(Flags{Root: dir})
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "larger than 1 MiB") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestValidateFailOn(t *testing.T) {
	for _, v := range []string{"error", "warning", "info", "never"} {
		if err := ValidateFailOn(v); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	err := ValidateFailOn("warn")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `did you mean "warning"`) {
		t.Errorf("err = %v", err)
	}
}

func TestDiscoveryOptions(t *testing.T) {
	f, err := Parse([]byte(fullConfig), "c.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s := Merge(Defaults(), f, Flags{})
	s.Root = "/repo"
	opts := s.DiscoveryOptions()
	if opts.Root != "/repo" || !slices.Equal(opts.Include, []string{"apps/**"}) || len(opts.Exclude) != 2 {
		t.Errorf("opts = %+v", opts)
	}
	if len(opts.Environments) != 1 || opts.Environments[0].Name != "production" {
		t.Errorf("Environments = %+v", opts.Environments)
	}
	if !slices.Equal(opts.Languages, []discovery.Language{discovery.LangGo, discovery.LangTypeScript}) {
		t.Errorf("Languages = %v", opts.Languages)
	}
}

// TestJSONSchemaInSync fails when docs/envguard.schema.json differs from the
// schema generated from the Go structs. Regenerate with:
//
//	go test ./internal/config -run TestJSONSchemaInSync -update
func TestJSONSchemaInSync(t *testing.T) {
	got, err := JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	testutil.Golden(t, filepath.Join("..", "..", "docs", "envguard.schema.json"), got)
}

// TestSchemaCoversStructs cross-checks that every yaml key of every config
// struct appears in the schema, and that enum sets are valid.
func TestSchemaCoversStructs(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[File](), reflect.TypeFor[Environment](), reflect.TypeFor[IgnoreEntry](), reflect.TypeFor[Scanner](), reflect.TypeFor[Variable]()} {
		s := schemaFor(typ, "")
		props, ok := s["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no properties", typ)
		}
		for name, f := range yamlFields(typ) {
			if _, ok := props[name]; !ok {
				t.Errorf("%s.%s missing from schema", typ.Name(), name)
			}
			if f.Tag.Get("desc") == "" {
				t.Errorf("%s.%s has no desc tag", typ.Name(), name)
			}
			if e := f.Tag.Get("enum"); e != "" && len(enumValues(e)) == 0 {
				t.Errorf("%s.%s: empty enum %s", typ.Name(), name, e)
			}
		}
	}
}

// TestSchemaValidationErrors covers strict validation of the variables:
// section: every problem is reported with file:line:col and exit code 2.
func TestSchemaValidationErrors(t *testing.T) {
	tests := []struct{ name, vars, want string }{
		{"unknown type", "A: { type: integer }", `c.yaml:3:14: invalid value for "variables.A.type"`},
		{"unknown type suggestion", "A: { type: boolean }", `(did you mean "bool"?)`},
		{"values without enum", "A: { values: [a] }", `"values" only applies to type enum`},
		{"schemes without url", "A: { type: string, schemes: [https] }", `"schemes" only applies to type url`},
		{"min on string", "A: { min: 1 }", `"min" and "max" only apply to types int, float and port`},
		{"min greater than max", "A: { type: int, min: 5, max: 1 }", `"min" is greater than "max"`},
		{"invalid regex", `A: { pattern: "a(" }`, `invalid regular expression`},
		{"default wrong type", `A: { type: int, default: abc }`, `"variables.A": default expected an integer`},
		{"default below min", `A: { type: int, min: 2, default: "1" }`, `default is below the minimum 2`},
		{"default not in enum", `A: { type: enum, values: [a, b], default: c }`, `default expected one of a or b`},
		{"default bad port", `A: { type: port, default: "70000" }`, `default expected a port number between 1 and 65535`},
		{"must_be wrong type", `A: { type: bool, production: { must_be: maybe } }`, `must_be expected a boolean`},
		{"must_be equals must_not_be", `A: { type: bool, production: { must_be: "off", must_not_be: "false" } }`, `"must_be" and "must_not_be" are equal`},
		{"empty production rule", `A: { production: {} }`, `set at least one of "must_be", "must_not_be" or "pattern"`},
		{"production bad regex", `A: { production: { pattern: "[" } }`, `invalid regular expression`},
		{"environments without required", `A: { environments: [production] }`, `"environments" has no effect unless "required: true" is set`},
		{"bad environment name", `A: { required: true, environments: [Prod] }`, `invalid environment name`},
		{"unknown key", `A: { type: int, maximum: 3 }`, `unknown key "maximum" in "variables.A" (did you mean "max"?)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte("version: 1\nvariables:\n  "+tt.vars+"\n"), "c.yaml")
			if err == nil {
				t.Fatal("want error")
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q\ndoes not contain %q", err, tt.want)
			}
		})
	}
	for _, typ := range []string{"string", "int", "float", "bool", "url", "port", "email", "hostname", "duration", "json", "regex"} {
		if _, err := Parse([]byte("version: 1\nvariables:\n  A: { type: "+typ+" }\n"), "c.yaml"); err != nil {
			t.Errorf("type %s rejected: %v", typ, err)
		}
	}
}

// A duplicate key is rejected by the YAML decoder too; the user must see one
// located, one-line problem and not the decoder's multi-line text.
func TestDuplicateKeyReportsOneLocatedProblem(t *testing.T) {
	_, err := Parse([]byte("version: 1\nversion: 1\n"), "c.yaml")
	var ce *Error
	if !errors.As(err, &ce) || len(ce.Problems) != 1 {
		t.Fatalf("want exactly one problem, got %v", err)
	}
	if got := ce.Problems[0].String(); strings.ContainsAny(got, "\n\r") || !strings.HasPrefix(got, "c.yaml:2:1: duplicate key") {
		t.Errorf("problem = %q", got)
	}
}

func TestDotsInGlobNamesAreFine(t *testing.T) {
	if _, err := Parse([]byte("version: 1\nexclude: [\"a..b/**\", \"**/*.min..js\", \"...\"]\n"), "c.yaml"); err != nil {
		t.Errorf("names containing dots must be accepted: %v", err)
	}
}
