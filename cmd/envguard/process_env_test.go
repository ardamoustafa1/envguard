package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
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

// unsetenv removes a variable for the duration of the test (t.Setenv
// registers the restore).
func unsetenv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func processEnvRepo(t *testing.T) string {
	return writeTree(t, map[string]string{
		"package.json": "{}",
		".env.example": "EGT_DATABASE_URL=\nEGT_PORT=3000\nEGT_OPTIONAL= # optional\nEGT_CACHE_URL=\n",
		".envguard.yaml": `version: 1
profiles: [production-safety]
variables:
  EGT_WORKERS: { type: int, min: 1, max: 64, required: true }
  EGT_API_KEY: { type: url, sensitive: true }
`,
		// Real env files are ignored in --process-env mode, even broken ones.
		".env":                  "BROKEN LINE\nEGT_DATABASE_URL=\n",
		".env.production":       "UNDOCUMENTED=1\n",
		"apps/api/package.json": "{}",
		"apps/api/.env.example": "EGT_API_ONLY=\n",
	})
}

func TestProcessEnvFindings(t *testing.T) {
	dir := processEnvRepo(t)
	unsetenv(t, "EGT_DATABASE_URL")
	t.Setenv("EGT_PORT", "")
	t.Setenv("EGT_WORKERS", "0")
	t.Setenv("EGT_API_KEY", "not-a-url-but-secret-ish-value")
	t.Setenv("EGT_CACHE_URL", "redis://localhost:6379")
	t.Setenv("NODE_ENV", "development")
	t.Setenv("SOME_RUNNER_URL", "http://localhost:9999") // undeclared: ignored

	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", dir, "check", "--process-env", "--env", "production"}, &stdout, &stderr)
	out := stdout.String()
	if code != exitFindings {
		t.Fatalf("exit %d\n%s%s", code, out, stderr.String())
	}
	for _, want := range []string{
		"EG001  EGT_DATABASE_URL is declared in .env.example but not set in the process environment",
		"EG005  EGT_PORT is empty in the process environment",
		"EG014  EGT_WORKERS in the process environment has the invalid value 0: is below the minimum 1",
		"EG014  EGT_API_KEY in the process environment has an invalid value: expected a URL",
		"EG015  NODE_ENV is development, but production must use",
		"EG015  EGT_CACHE_URL points to a local address in production",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"EG002", "EG007", "EG008", "EG003", "SOME_RUNNER_URL", "not-a-url", "EGT_API_ONLY", "EGT_OPTIONAL"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("unexpected %q in:\n%s", unwanted, out)
		}
	}
}

func TestProcessEnvClean(t *testing.T) {
	dir := processEnvRepo(t)
	t.Setenv("EGT_DATABASE_URL", "postgres://db.internal/app")
	t.Setenv("EGT_PORT", "8080")
	t.Setenv("EGT_WORKERS", "4")
	t.Setenv("EGT_CACHE_URL", "redis://cache.internal:6379")
	t.Setenv("NODE_ENV", "production")
	unsetenv(t, "EGT_API_KEY")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", dir, "check", "--process-env", "--env", "prod", "--quiet"}, &stdout, &stderr)
	if code != exitOK || stdout.Len() != 0 {
		t.Errorf("exit %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestProcessEnvApp(t *testing.T) {
	dir := processEnvRepo(t)
	unsetenv(t, "EGT_API_ONLY")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", dir, "check", "--process-env", "--app", "./apps/api"}, &stdout, &stderr)
	if code != exitFindings || !strings.Contains(stdout.String(), "EGT_API_ONLY is declared in apps/api/.env.example") {
		t.Errorf("exit %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "EGT_DATABASE_URL") {
		t.Error("--app apps/api must not use the root app's templates")
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--root", dir, "check", "--app", "nope"}, &stdout, &stderr); code != exitError ||
		!strings.Contains(stderr.String(), "known app roots: ., apps/api") || strings.Contains(stderr.String(), "nope") {
		t.Errorf("bad --app: exit %d, stderr %q", code, stderr.String())
	}
	if code := run([]string{"--root", dir, "check", "--process-env", "--env", "a,b"}, &stdout, &stderr); code != exitError {
		t.Errorf("two environments with --process-env: exit %d", code)
	}
}

// TestAppFilterInFileMode: --app without --process-env keeps only findings
// of that app root.
func TestAppFilterInFileMode(t *testing.T) {
	dir := processEnvRepo(t)
	var stdout, stderr bytes.Buffer
	run([]string{"--root", dir, "check", "--app", "apps/api", "--fail-on", "never"}, &stdout, &stderr)
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if strings.Contains(line, "  ") && !strings.HasPrefix(line, "apps/api/") {
			t.Errorf("finding outside apps/api: %s", line)
		}
	}
}
