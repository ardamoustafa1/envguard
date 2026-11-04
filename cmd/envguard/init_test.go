package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/testutil"
)

const initFixture = "testdata/init-project"

// TestInitGolden snapshots `init --dry-run --example`. Regenerate with:
// go test ./cmd/envguard -run TestInitGolden -update
func TestInitGolden(t *testing.T) {
	dir := testutil.CopyFixture(t, initFixture)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", dir, "init", "--dry-run", "--example"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	testutil.Golden(t, filepath.Join("testdata", "init.golden"), stdout.Bytes())
	if _, err := os.Stat(filepath.Join(dir, ".envguard.yaml")); err == nil {
		t.Error("--dry-run wrote a file")
	}
}

// TestInitNeverCopiesValues proves that no value from a real env file or a
// template ends up in anything init writes or prints.
func TestInitNeverCopiesValues(t *testing.T) {
	dir := testutil.CopyFixture(t, initFixture)
	// Add canaries in every value position at runtime.
	var env, tpl strings.Builder
	for i := range 20 {
		env.WriteString("CANARY_VAR_" + string(rune('A'+i)) + "=" + testutil.Canary(i) + "\n")
		tpl.WriteString("CANARY_TPL_" + string(rune('A'+i)) + "=" + testutil.Canary(100+i) + "\n")
	}
	appendFile(t, filepath.Join(dir, ".env"), env.String())
	appendFile(t, filepath.Join(dir, ".env.example"), tpl.String())
	appendFile(t, filepath.Join(dir, "src", "app.ts"), "const c = process.env.CANARY_VAR_A;\n")

	realValues := []string{"REAL_ENV_VALUE", "TEMPLATE_VALUE_MUST_NOT_BE_COPIED", "CNRY", "localhost:6379", "0.5", "your-stripe-key"}
	envData, _ := os.ReadFile(filepath.Join(dir, ".env"))
	tplData, _ := os.ReadFile(filepath.Join(dir, ".env.example"))
	for _, data := range [][]byte{envData, tplData} {
		for _, line := range strings.Split(string(data), "\n") {
			if _, v, ok := strings.Cut(line, "="); ok && len(v) >= 6 {
				realValues = append(realValues, v)
			}
		}
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", dir, "init", "--example", "--force"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	written, err := os.ReadFile(filepath.Join(dir, ".envguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	example, err := os.ReadFile(filepath.Join(dir, ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	var dry bytes.Buffer
	run([]string{"--root", dir, "init", "--dry-run", "--example"}, &dry, &stderr)

	for name, out := range map[string]string{
		".envguard.yaml": string(written), ".env.example": string(example),
		"stdout": stdout.String(), "dry-run": dry.String(), "stderr": stderr.String(),
	} {
		for _, v := range realValues {
			if strings.Contains(out, v) {
				t.Errorf("%s contains a copied value %q", name, v)
			}
		}
		for _, line := range strings.Split(out, "\n") {
			if name == ".env.example" && strings.Contains(line, "=") && !strings.HasPrefix(line, "#") {
				if _, v, _ := strings.Cut(line, "="); strings.TrimSpace(strings.Split(v, "#")[0]) != "" {
					t.Errorf(".env.example has a non-empty value: %q", line)
				}
			}
		}
	}
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

func TestInitWritesValidConfigAndRefusesOverwrite(t *testing.T) {
	dir := testutil.CopyFixture(t, initFixture)
	if err := os.Remove(filepath.Join(dir, ".env.example")); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", dir, "init", "--example"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("first init: exit %d: %s", code, stderr.String())
	}
	for _, want := range []string{"Wrote .envguard.yaml:", "Wrote .env.example:", "Next steps:"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, stdout.String())
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".envguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Parse(data, ".envguard.yaml"); err != nil {
		t.Fatalf("generated config is invalid: %v\n%s", err, data)
	}
	// The generated config is usable by check (no configuration error).
	stdout.Reset()
	if code := run([]string{"--root", dir, "check", "--fail-on", "never"}, &stdout, &stderr); code != exitOK {
		t.Errorf("check with generated config: exit %d: %s", code, stderr.String())
	}

	// A second run refuses to overwrite.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--root", dir, "init"}, &stdout, &stderr); code != exitError ||
		!strings.Contains(stderr.String(), ".envguard.yaml already exists; use --force") {
		t.Errorf("overwrite: exit %d, stderr %q", code, stderr.String())
	}
	// --force overwrites the config but keeps .env.example unless asked.
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("KEEP_ME=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if code := run([]string{"--root", dir, "init", "--force"}, &stdout, &stderr); code != exitOK {
		t.Errorf("--force: exit %d: %s", code, stderr.String())
	}
	if code := run([]string{"--root", dir, "init", "--force", "--example"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "Wrote .env.example") {
		t.Errorf("--force --example should regenerate .env.example:\n%s", stdout.String())
	}
}

func TestInitKeepsExistingExample(t *testing.T) {
	dir := testutil.CopyFixture(t, initFixture)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", dir, "init", "--example"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Kept the existing .env.example") {
		t.Errorf("stdout:\n%s", stdout.String())
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".env.example"))
	if !strings.Contains(string(data), "DATABASE_URL=") || strings.Contains(string(data), "Generated by") {
		t.Error("existing .env.example was modified")
	}
}

func TestInitEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", dir, "init", "--dry-run"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "No variables found yet") {
		t.Errorf("stdout:\n%s", stdout.String())
	}
	cfg := strings.TrimPrefix(stdout.String(), "# --- .envguard.yaml ---\n")
	if _, err := config.Parse([]byte(cfg), "c.yaml"); err != nil {
		t.Errorf("empty config invalid: %v", err)
	}
}
