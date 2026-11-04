package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Monorepo benchmark: 40 apps x 500 source files = 20,000 source files and
// 5 env files per app = 200 env files, 40 Compose files, a workflow, tracked
// in git. Run it with
//
//	go test ./cmd/envguard -run '^$' -bench Monorepo -benchtime 3x -cpuprofile cpu.out
//
// and ENVGUARD_WRITE_BENCH=<empty dir> go test ./cmd/envguard -run TestWriteBenchRepo
// writes the repository so the real binary can be timed on it.
const (
	benchApps         = 40
	benchFilesPerApp  = 500
	benchVarsPerApp   = 30
	benchPaddingLines = 60
)

var benchTemplates = map[string]string{
	".ts": "import { db } from \"./db\";\n// process.env.FAKE_IN_COMMENT\nconst url = process.env.APP_VAR_%[2]d;\nconst port = process.env.PORT || 3000;\nconst { APP_VAR_%[3]d, TIMEOUT = \"30\" } = process.env;\nconst msg = \"process.env.FAKE_IN_STRING\";\nexport const h = (r: Request) => new Response(JSON.stringify({ url, port, msg, r }));\n",
	".js": "const express = require('express');\n/* process.env.FAKE_IN_BLOCK */\nconst app = express();\napp.listen(process.env.APP_VAR_%[2]d || 3000);\nmodule.exports = { token: process.env['APP_VAR_%[3]d'] };\n",
	".py": "\"\"\"Docstring mentioning os.getenv('FAKE').\"\"\"\nimport os\n# os.environ[\"FAKE_IN_COMMENT\"]\nURL = os.environ[\"APP_VAR_%[2]d\"]\nDEBUG = os.getenv(\"APP_VAR_%[3]d\", \"false\")\ndef handler(event):\n    return {\"url\": URL, \"debug\": DEBUG, \"text\": \"os.getenv('FAKE')\"}\n",
	".go": "package svc\n\nimport \"os\"\n\n// os.Getenv(\"FAKE_IN_COMMENT\")\ntype Config struct {\n\tPort int `env:\"APP_VAR_%[2]d\" envDefault:\"8080\"`\n}\n\nfunc Load() string { return os.Getenv(\"APP_VAR_%[3]d\") + \"os.Getenv(\\\"FAKE\\\")\" }\n",
	".rb": "# ENV[\"FAKE_IN_COMMENT\"]\nclass Svc%[1]d\n  def url = ENV.fetch(\"APP_VAR_%[2]d\")\n  def port = ENV[\"APP_VAR_%[3]d\"] || 3000\n  def text = \"ENV['FAKE']\"\nend\n",
	".sh": "#!/usr/bin/env bash\nset -euo pipefail\n# $FAKE_IN_COMMENT\nTARGET=prod\necho \"deploying ${APP_VAR_%[2]d} to $TARGET with ${APP_VAR_%[3]d:-eu}\"\necho '$FAKE_IN_SINGLE_QUOTES'\n",
}

func writeMonorepo(tb testing.TB, root string) {
	tb.Helper()
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			tb.Fatal(err)
		}
	}
	exts := []string{".ts", ".js", ".py", ".go", ".rb", ".sh"}
	padding := strings.Repeat("function helper(a, b) { return a + b; } // padding line with some text to look real\n", benchPaddingLines)
	write(".gitignore", ".env\n.env.*\n!.env.*.example\n!.env.example\nnode_modules/\n")
	write("package.json", `{"name":"monorepo","private":true}`)
	write(".github/workflows/ci.yml", "on: push\njobs:\n  t:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ${{ secrets.DEPLOY_TOKEN }}\n")
	for a := range benchApps {
		app := fmt.Sprintf("apps/app%02d", a)
		write(app+"/package.json", fmt.Sprintf(`{"name":"app%02d"}`, a))
		var tmpl, live strings.Builder
		for v := range benchVarsPerApp {
			fmt.Fprintf(&tmpl, "APP_VAR_%d=\n", v)
			fmt.Fprintf(&live, "APP_VAR_%d=value%d\n", v, v)
		}
		write(app+"/.env.example", tmpl.String())
		write(app+"/.env", live.String())
		write(app+"/.env.production.example", tmpl.String()+"PROD_ONLY=\n")
		write(app+"/.env.production", live.String()+"PROD_ONLY=1\n")
		write(app+"/.env.staging.example", tmpl.String())
		write(app+"/docker-compose.yml", fmt.Sprintf("services:\n  app:\n    image: app%02d\n    environment:\n      APP_VAR_0: ${APP_VAR_0}\n    env_file: [.env]\n", a))
		for f := range benchFilesPerApp {
			ext := exts[f%len(exts)]
			body := fmt.Sprintf(benchTemplates[ext], f, f%benchVarsPerApp, (f+7)%benchVarsPerApp) + padding
			write(fmt.Sprintf("%s/src/m%02d/file%04d%s", app, f/50, f, ext), body)
		}
	}
}

func stageInGit(tb testing.TB, root string) {
	tb.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	for _, args := range [][]string{{"-c", "init.defaultBranch=main", "init", "-q"}, {"add", "-A"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...) //nolint:gosec // test
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func BenchmarkCheckMonorepo(b *testing.B) {
	root := b.TempDir()
	writeMonorepo(b, root)
	stageInGit(b, root)
	var out bytes.Buffer
	if code := run([]string{"--root", root, "check", "--fail-on", "never", "--format", "json"}, &out, &out); code != exitOK {
		b.Fatalf("exit %d", code)
	}
	b.Logf("report: %d bytes", out.Len())
	b.ResetTimer()
	for b.Loop() {
		var stdout, stderr bytes.Buffer
		run([]string{"--root", root, "check", "--fail-on", "never", "--format", "json"}, &stdout, &stderr)
	}
}

func BenchmarkDoctorMonorepo(b *testing.B) {
	root := b.TempDir()
	writeMonorepo(b, root)
	stageInGit(b, root)
	b.ResetTimer()
	for b.Loop() {
		var stdout, stderr bytes.Buffer
		run([]string{"--root", root, "doctor"}, &stdout, &stderr)
	}
}

// TestMonorepoSmoke keeps the generator honest at small scale.
func TestWriteBenchRepo(t *testing.T) {
	dest := os.Getenv("ENVGUARD_WRITE_BENCH")
	if dest == "" {
		t.Skip("set ENVGUARD_WRITE_BENCH=<empty dir> to write the benchmark repository")
	}
	writeMonorepo(t, dest)
	stageInGit(t, dest)
}
