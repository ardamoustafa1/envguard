package rules

import (
	"slices"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// Regression tests for false positives found by running EnvGuard on
// realistic projects (testdata/realworld) before v0.1.0.

func severities(fs []findings.Finding) map[string]findings.Severity {
	out := map[string]findings.Severity{}
	for _, f := range fs {
		out[f.Variable] = f.Severity
	}
	return out
}

func TestEG003Severity(t *testing.T) {
	const pkg = `{"name":"app"}`
	tests := []struct {
		name  string
		files map[string]string
		want  map[string]findings.Severity
	}{
		{"required read in a repo that declares other variables is an error",
			map[string]string{"package.json": pkg, ".env.example": "OTHER=\n", "a.js": "x(process.env.NEEDED); y(process.env.OTHER)"},
			map[string]findings.Severity{"NEEDED": findings.Error}},
		{"a default makes it info: the app works without it",
			map[string]string{"package.json": pkg, ".env.example": "OTHER=\n", "a.js": "x(process.env.PORT_X || 3000)"},
			map[string]findings.Severity{"PORT_X": findings.Info}},
		{"a comparison is a handled absence",
			map[string]string{"package.json": pkg, ".env.example": "OTHER=\n", "a.js": `if (process.env.ANALYZE === "true") {}`},
			map[string]findings.Severity{"ANALYZE": findings.Info}},
		{"a repo that declares nothing (a CLI or library) gets suggestions, not errors",
			map[string]string{"go.mod": "module x", "main.go": "package main\nvar a = os.Getenv(\"TOOL_DIR\")\nvar b = os.Getenv(\"TOOL_COLOR\")"},
			map[string]findings.Severity{"TOOL_DIR": findings.Info, "TOOL_COLOR": findings.Info}},
		{"any declaration anywhere (here a workflow env) makes the repo a configured one",
			map[string]string{"go.mod": "module x", "main.go": "package main\nvar a = os.Getenv(\"TOOL_DIR\")",
				".github/workflows/ci.yml": "on: push\nenv:\n  SOMETHING: 1\njobs: {}\n"},
			map[string]findings.Severity{"TOOL_DIR": findings.Error}},
		{"Laravel env() never fails, so it is optional",
			map[string]string{"composer.json": "{}", ".env.example": "APP_NAME=x\n", "config/db.php": "<?php return ['url' => env('DB_URL')];"},
			map[string]findings.Severity{"DB_URL": findings.Info}},
		{"ambient and platform variables are never reported",
			map[string]string{"package.json": pkg, ".env.example": "OTHER=\n",
				"a.js": "a(process.env.VERCEL); b(process.env.VITEST); c(process.env.XDG_CONFIG_HOME); d(process.env.LC_ALL); e(process.env.EDITOR)"},
			map[string]findings.Severity{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := severities(runRule(t, tree(t, tt.files), "EG003"))
			if len(got) != len(tt.want) {
				t.Fatalf("findings = %v, want %v", got, tt.want)
			}
			for v, sev := range tt.want {
				if got[v] != sev {
					t.Errorf("%s: severity %v, want %v (all: %v)", v, got[v], sev, got)
				}
			}
		})
	}
}

// Inferred names (Go envPrefix) are guesses: they must never make EG003 claim
// a variable is undeclared, but they do count as reads for EG004.
func TestInferredNamesCountAsReadsButNeverAsUndeclared(t *testing.T) {
	files := map[string]string{
		"go.mod":       "module x",
		"cfg.go":       "package cfg\ntype C struct {\n\tDB DB `envPrefix:\"DB_\"`\n}\ntype DB struct {\n\tURL string `env:\"URL\"`\n\tPool int `env:\"POOL\"`\n}\n",
		".env.example": "DB_URL=\nDB_POOL=\nDB_UNUSED=\n",
	}
	dir := tree(t, files)
	if got := runRule(t, dir, "EG003"); len(got) != 0 {
		t.Errorf("EG003 on inferred names: %v", vars(got))
	}
	got := vars(runRule(t, dir, "EG004"))
	if !slices.Equal(got, []string{"DB_UNUSED"}) {
		t.Errorf("EG004 = %v, want only DB_UNUSED (DB_URL and DB_POOL are read through the prefix)", got)
	}
}

// When the whole environment is handed to a validator, the names live in a
// schema: EG004 cannot know what is unused and must stay silent.
func TestEG004SilentWhenTheWholeEnvironmentIsRead(t *testing.T) {
	files := map[string]string{
		"package.json": `{"name":"app"}`, ".env.example": "A=\nB=\n",
		"src/env.ts": "export const env = schema.parse(process.env);\n",
	}
	dir := tree(t, files)
	if got := runRule(t, dir, "EG004"); len(got) != 0 {
		t.Errorf("EG004 reported %v although process.env is read as a whole", vars(got))
	}
	files["src/env.ts"] = "export const env = process.env.A;\n"
	if got := vars(runRule(t, tree(t, files), "EG004")); !slices.Equal(got, []string{"B"}) {
		t.Errorf("without the bulk read EG004 = %v, want [B]", got)
	}
	// Passing the environment to a child process says nothing about names.
	files["src/env.ts"] = "spawn('x', { env: { ...process.env, Z: '1' } }); a(process.env.A);\n"
	if got := vars(runRule(t, tree(t, files), "EG004")); !slices.Equal(got, []string{"B"}) {
		t.Errorf("a pass-through spread must not silence EG004: %v", got)
	}
}

// django-environ settings are split over modules; only one imports environ.
func TestDjangoEnvironReadsInModulesThatDoNotImportIt(t *testing.T) {
	files := map[string]string{
		"pyproject.toml":  "[project]\ndependencies = [\"django-environ\"]\n",
		"config/base.py":  "import environ\nenv = environ.Env()\n",
		"config/prod.py":  "from .base import env\nSECRET_KEY = env(\"DJANGO_SECRET_KEY\")\nDEBUG = env.bool(\"DJANGO_DEBUG\", default=False)\n",
		"config/other.py": "import os\nTOKEN = os.environ[\"OTHER_TOKEN\"]\n", // keeps EG004 active: the app has some usage
		".env.example":    "DJANGO_SECRET_KEY=\nDJANGO_DEBUG=\nNOT_READ=\nOTHER_TOKEN=\n",
	}
	got := vars(runRule(t, tree(t, files), "EG004"))
	if !slices.Equal(got, []string{"NOT_READ"}) {
		t.Errorf("EG004 = %v, want [NOT_READ]", got)
	}
	delete(files, "pyproject.toml")
	got = vars(runRule(t, tree(t, files), "EG004"))
	if !strings.Contains(strings.Join(got, ","), "DJANGO_SECRET_KEY") {
		t.Errorf("without the dependency the bare env() calls must not be trusted: %v", got)
	}
}
