package rules

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// tree writes files into a fresh directory and returns it.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runRule(t *testing.T, dir, id string) []findings.Finding {
	t.Helper()
	p := loadProject(t, dir, nil)
	res, err := Run(p, []Rule{Lookup1(t, id)})
	if err != nil {
		t.Fatal(err)
	}
	return res.Findings
}

func vars(fs []findings.Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Variable)
	}
	return out
}

// TestProductionSafetyProfile checks each framework variable of the
// production-safety profile on its own.
func TestProductionSafetyProfile(t *testing.T) {
	tests := []struct {
		line   string
		report bool
	}{
		{"DEBUG=true", true}, {"DEBUG=1", true}, {"DEBUG=ON", true}, {"DEBUG=false", false}, {"DEBUG=", false},
		{"APP_DEBUG=true", true}, {"APP_DEBUG=false", false},
		{"FLASK_DEBUG=1", true}, {"FLASK_DEBUG=0", false},
		{"DJANGO_DEBUG=True", true}, {"DJANGO_DEBUG=False", false},
		{"NODE_ENV=development", true}, {"NODE_ENV=test", true}, {"NODE_ENV=production", false},
		{"RAILS_ENV=staging", true}, {"RAILS_ENV=production", false},
		{"APP_ENV=local", true}, {"APP_ENV=development", true}, {"APP_ENV=dev", true}, {"APP_ENV=production", false}, {"APP_ENV=staging", false},
		{"DATABASE_URL=postgres://localhost/app", true}, {"DATABASE_URL=postgres://127.0.0.1:5432/app", true},
		{"API_URL=http://0.0.0.0:3000", true}, {"API_URL=http://[::1]:3000", true}, {"DATABASE_URL=postgres://db.internal/app", false},
		{"REDIS_HOST=localhost", true}, {"REDIS_HOST=127.0.0.1:6379", true}, {"REDIS_HOST=::1", true}, {"REDIS_HOST=redis.internal", false},
		{"LOCALHOST_NAME=localhost", false}, // neither *_URL nor *_HOST
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			dir := tree(t, map[string]string{
				".envguard.yaml":  "version: 1\nprofiles: [production-safety]\n",
				".env.production": tt.line + "\n",
			})
			got := runRule(t, dir, "EG015")
			if (len(got) > 0) != tt.report {
				t.Errorf("findings = %v, want report=%v", vars(got), tt.report)
			}
		})
	}
}

func TestProductionSafetyIsOptIn(t *testing.T) {
	dir := tree(t, map[string]string{".env.production": "DEBUG=true\nNODE_ENV=development\n"})
	if got := runRule(t, dir, "EG015"); len(got) != 0 {
		t.Errorf("profile must be off by default: %v", vars(got))
	}
	dir = tree(t, map[string]string{
		".envguard.yaml":   "version: 1\nprofiles: [production-safety]\n",
		".env.staging":     "DEBUG=true\n",
		".env.development": "DEBUG=true\n",
		".env.prod":        "DEBUG=true\n", // prod is normalized to production
	})
	got := runRule(t, dir, "EG015")
	if len(got) != 1 || got[0].File != ".env.prod" {
		t.Errorf("only production files count: %+v", got)
	}
}

func TestSchemaRequired(t *testing.T) {
	cfg := `version: 1
variables:
  DATABASE_URL: { required: true, type: url, environments: [staging, production] }
  SECRET_KEY: { required: true, sensitive: true }
  OPTIONAL_THING: { type: int }
`
	dir := tree(t, map[string]string{
		".envguard.yaml":        cfg,
		"package.json":          "{}",
		".env":                  "OPTIONAL_THING=1\n",
		".env.production":       "SECRET_KEY=\n",
		"apps/api/package.json": "{}",
		"apps/api/.env":         "UNRELATED=1\n",
		"apps/web/package.json": "{}",
		"apps/web/.env.example": "SECRET_KEY=\n",
		"apps/web/.env":         "X=1\n",
	})
	got := runRule(t, dir, "EG001")
	var keys []string
	for _, f := range got {
		keys = append(keys, f.File+":"+f.Variable)
	}
	slices.Sort(keys)
	// DATABASE_URL only in production; SECRET_KEY everywhere, but only in app
	// roots that reference it (root app always; apps/web via its template;
	// not apps/api). apps/web's SECRET_KEY comes from its template.
	want := []string{".env.production:DATABASE_URL", ".env:SECRET_KEY", "apps/web/.env:SECRET_KEY"}
	if !slices.Equal(keys, want) {
		t.Errorf("EG001 = %v\nwant    %v", keys, want)
	}
	for _, f := range got {
		if f.Variable == "DATABASE_URL" && !strings.Contains(f.Message, "required by the schema (.envguard.yaml:3)") {
			t.Errorf("message = %q", f.Message)
		}
	}

	empty := runRule(t, dir, "EG005")
	if len(empty) != 1 || empty[0].Variable != "SECRET_KEY" || empty[0].File != ".env.production" {
		t.Errorf("EG005 = %+v", empty)
	}

	// Schema-declared variables are documented for EG002.
	undoc := runRule(t, dir, "EG002")
	for _, f := range undoc {
		if f.Variable == "OPTIONAL_THING" || f.Variable == "SECRET_KEY" {
			t.Errorf("schema variable reported as undocumented: %s", f.Message)
		}
	}
}

func TestSensitiveSchemaMasksEverywhere(t *testing.T) {
	dir := tree(t, map[string]string{
		".envguard.yaml": "version: 1\nvariables:\n  PUBLIC_LABEL: { sensitive: true, type: int }\n",
		".env":           "PUBLIC_LABEL=hello\nPUBLIC_LABEL=world\n",
	})
	for _, id := range []string{"EG006", "EG014"} {
		for _, f := range runRule(t, dir, id) {
			for _, text := range []string{f.Message, f.Evidence, f.Fix, f.Why} {
				if strings.Contains(text, "hello") || strings.Contains(text, "world") {
					t.Errorf("%s shows a sensitive value: %q", id, text)
				}
			}
		}
	}
}
