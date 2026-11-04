package main

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// forbiddenImports must never be imported by production code: the network
// (principle 1) and telemetry or crash-reporting clients. "prefix/" entries
// match whole import-path prefixes.
var forbiddenImports = []string{
	// Exact packages. Note "net" itself pulls in sockets and the resolver;
	// the pure parsers net/url and net/netip are fine.
	"net", "net/http", "net/rpc", "net/smtp", "net/mail", "net/textproto", "crypto/tls", "expvar", "plugin",
	// Everything below these paths.
	"net/http/", "net/rpc/", "net/smtp/", "net/mail/",
	"go.opentelemetry.io/", "github.com/getsentry/", "github.com/segmentio/analytics-go", "github.com/posthog/",
	"github.com/DataDog/", "github.com/newrelic/", "github.com/honeycombio/", "github.com/amplitude/",
	"github.com/mixpanel/", "github.com/bugsnag/", "github.com/rollbar/", "github.com/prometheus/client_golang",
	"github.com/denisbrodbeck/machineid", "github.com/matishsiao/goInfo",
	"cloud.google.com/", "google.golang.org/grpc", "github.com/aws/aws-sdk-go",
}

// forbiddenModules are modules go.mod must not require, directly or not.
var forbiddenModules = []string{
	"go.opentelemetry.io/", "github.com/getsentry/", "github.com/segmentio/analytics-go", "github.com/posthog/",
	"github.com/DataDog/", "github.com/newrelic/", "github.com/honeycombio/", "github.com/amplitude/",
	"github.com/mixpanel/", "github.com/bugsnag/", "github.com/rollbar/", "google.golang.org/grpc",
	"github.com/aws/aws-sdk-go", "cloud.google.com/",
}

// importForbidden: entries ending in "/" match everything below that path;
// other entries match exactly, except module paths, which also match their
// subpackages and major-version suffixes.
func importForbidden(p string) bool {
	for _, f := range forbiddenImports {
		switch {
		case strings.HasSuffix(f, "/"):
			if strings.HasPrefix(p, f) {
				return true
			}
		case strings.Contains(f, "."): // a module path: subpackages and /vN too
			if p == f || strings.HasPrefix(p, f+"/") {
				return true
			}
		case p == f:
			return true
		}
	}
	return false
}

// goSources walks the module's production Go files (no tests, no testdata).
func goSources(t *testing.T, visit func(rel string, imports []string)) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", "node_modules", "dist", "bin", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoRoot, p)
		var imps []string
		for _, im := range f.Imports {
			path, err := strconv.Unquote(im.Path.Value)
			if err != nil {
				return err
			}
			imps = append(imps, path)
		}
		n++
		visit(filepath.ToSlash(rel), imps)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestNoNetworkOrTelemetryImports parses the imports of every production Go
// file. Unlike TestOffline (which inspects the whole dependency graph with
// `go list`), this works without the go tool and names the offending file.
func TestNoNetworkOrTelemetryImports(t *testing.T) {
	n := goSources(t, func(rel string, imports []string) {
		for _, imp := range imports {
			if importForbidden(imp) {
				t.Errorf("%s imports %q: EnvGuard must stay offline and free of telemetry (CLAUDE.md principle 1)", rel, imp)
			}
			// Running programs is confined to the git wrapper.
			if imp == "os/exec" && !strings.HasPrefix(rel, "internal/gitutil/") && !strings.HasPrefix(rel, "internal/testutil/") {
				t.Errorf("%s imports os/exec: only internal/gitutil may run programs", rel)
			}
		}
	})
	if n < 50 {
		t.Fatalf("only %d production Go files found; the walk is broken", n)
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		for _, m := range forbiddenModules {
			if strings.Contains(line, m) {
				t.Errorf("go.mod requires a forbidden module: %s", strings.TrimSpace(line))
			}
		}
	}
}

// TestImportCheckCatchesViolations makes sure the matcher itself works.
func TestImportCheckCatchesViolations(t *testing.T) {
	for _, bad := range []string{"net", "net/http", "net/http/httputil", "crypto/tls", "go.opentelemetry.io/otel",
		"github.com/getsentry/sentry-go", "github.com/segmentio/analytics-go/v3", "google.golang.org/grpc/credentials", "net/smtp"} {
		if !importForbidden(bad) {
			t.Errorf("%q should be forbidden", bad)
		}
	}
	for _, ok := range []string{"net/netip", "net/url", "os", "os/exec", "github.com/spf13/cobra", "net_test", "internal/network"} {
		if importForbidden(ok) {
			t.Errorf("%q should be allowed", ok)
		}
	}
}

func TestRunGuardedTurnsPanicsIntoErrors(t *testing.T) {
	canary := "CNRY0001-secret-value"
	var nilMap map[string]int
	var short []int
	idx := 3
	tests := []struct {
		name string
		fn   func() int
		want string
	}{
		{"string panic carrying a value", func() int { panic(canary) }, "unexpected panic of type string"},
		{"error panic carrying a value", func() int { panic(errors.New(canary)) }, "unexpected panic of type *errors.errorString"},
		{"runtime error", func() int { return short[idx] }, "index out of range [3] with length 0"},
		{"nil map write", func() int { nilMap[canary] = 1; return 0 }, "assignment to entry in nil map"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := runGuarded(&stderr, tt.fn)
			if code != exitError {
				t.Errorf("exit code %d, want %d", code, exitError)
			}
			if !strings.Contains(stderr.String(), tt.want) || !strings.Contains(stderr.String(), "please report it") {
				t.Errorf("stderr = %q", stderr.String())
			}
			if strings.Contains(stderr.String(), "CNRY") {
				t.Errorf("panic output leaks a value: %q", stderr.String())
			}
			if strings.Count(stderr.String(), "\n") != 1 {
				t.Errorf("want a one-line message, got %q", stderr.String())
			}
		})
	}
	var stderr bytes.Buffer
	if code := runGuarded(&stderr, func() int { return 1 }); code != 1 || stderr.Len() != 0 {
		t.Error("runGuarded must pass a normal result through")
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	dir := demoRepo(t)
	before := treeDigest(t, dir)
	for _, args := range [][]string{
		{"init", "--dry-run"}, {"init", "--dry-run", "--example"}, {"init", "--dry-run", "--force"},
		{"init", "--dry-run", "--force", "--example"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(append([]string{"--root", dir}, args...), &stdout, &stderr); code != exitOK {
			t.Errorf("envguard %s: exit %d: %s", strings.Join(args, " "), code, stderr.String())
		}
		if treeDigest(t, dir) != before {
			t.Fatalf("envguard %s changed the repository", strings.Join(args, " "))
		}
	}
}

// TestInitOverwriteNeedsForce: without --force init never touches existing
// files, byte for byte, and says what to do.
func TestInitOverwriteNeedsForce(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{".envguard.yaml": "version: 1\n# mine\n", ".env.example": "MINE=\n", "package.json": "{}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := treeDigest(t, dir)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", dir, "init", "--example"}, &stdout, &stderr); code != exitError {
		t.Errorf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "use --force") {
		t.Errorf("error does not say how to proceed: %q", stderr.String())
	}
	if treeDigest(t, dir) != before {
		t.Error("init changed existing files without --force")
	}
}

// TestComposeEnvFilePathsOutsideTheRoot: env_file entries that point outside
// the repository (../../etc/passwd, absolute paths, ${VARS}) are never
// stat'ed or reported; EnvGuard must not even reveal whether they exist.
func TestComposeEnvFilePathsOutsideTheRoot(t *testing.T) {
	outer := t.TempDir()
	if err := os.WriteFile(filepath.Join(outer, "secret.env"), []byte("OUTSIDE=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(outer, "repo")
	if err := os.MkdirAll(filepath.Join(root, "deploy"), 0o750); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  a:\n    image: x\n    env_file:\n" +
		"      - ../../etc/passwd\n      - ../../secret.env\n      - /etc/passwd\n      - ${HOME}/.env\n      - ../../../../../../nonexistent\n" +
		"      - path: ../../secret.env\n        required: true\n"
	if err := os.WriteFile(filepath.Join(root, "deploy", "docker-compose.yml"), []byte(compose), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--root", root, "check", "--format", "json", "--fail-on", "never"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	out := stdout.String() + stderr.String()
	for _, leak := range []string{"passwd", "secret.env", "nonexistent", "OUTSIDE", "does not exist"} {
		if strings.Contains(out, leak) {
			t.Errorf("output mentions %q; env_file paths outside the root must be ignored:\n%s", leak, out)
		}
	}
}

// hostileRepo builds a repository full of things that have broken tools like
// this one: huge, binary, deeply nested, invalid UTF-8, hostile links and
// filenames. Anything the platform cannot create is skipped.
func hostileRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel string, data []byte) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		// Windows cannot create names with control characters or newlines;
		// whatever the platform refuses is simply not part of the repository.
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Logf("cannot create %q here: %v", rel, err)
			return
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Logf("cannot create %q here: %v", rel, err)
		}
	}
	write("package.json", []byte(`{"name":"hostile","dependencies":{"next":"15"}}`))

	var big bytes.Buffer
	for i := range 3_000 {
		big.WriteString("KEY_" + strconv.Itoa(i) + "=value " + strconv.Itoa(i) + "\n")
	}
	write(".env.example", big.Bytes()[:60_000])                                      // ~3k keys
	write(".env.production", bytes.Repeat([]byte("A=1\n"), 1300<<10/4))              // 1.3 MiB: over the 1 MiB limit
	write(".env.staging", append(bytes.Repeat([]byte("K"), 60_000), '=', '1', '\n')) // one 60 kB key
	write(".env", []byte("A=\xff\xfe\nB=ok\n\xc3\x28=1\nC=\"\xe2\x82\n${\xff}=\nD=${\xc3}\n"))
	write(".env.local", []byte("\xef\xbb\xbfX=1\r\nY=2\r\n"))

	deep := strings.Repeat("d/", 40)
	write(deep+".env", []byte("DEEP=1\n"))
	write(deep+"deep.js", []byte("process.env.DEEP_VAR\n"))

	write("blob.js", append([]byte("process.env.IN_BINARY\x00\x00"), bytes.Repeat([]byte{0, 1, 2}, 1000)...))
	write("huge.js", bytes.Repeat([]byte("process.env.HUGE_VAR; "), 1100<<10/22))
	write("bad.py", []byte("import os\nos.environ[\"A\xff\"]\nos.getenv('\xc3(')\nf\"{os.environ['\xe2\x82']}\"\n"))
	write("bad.go", []byte("package p\nvar _ = os.Getenv(\"\xff\")\ntype C struct{ A string `env:\"\xc3\"` }\n"))
	write("bad.sh", []byte("#!/bin/sh\necho ${A\xff:-1} $\xc3\n"))
	write("bad.php", []byte("<?php getenv('\xff'); $_ENV['\xc3'];"))
	write("bad.rb", []byte("ENV['\xff']; =begin\n\xc3"))
	write("minified.js", append(bytes.Repeat([]byte("a=process.env.MIN_"), 1), bytes.Repeat([]byte("x"), 300<<10)...))

	write("docker-compose.yml", []byte("services:\n  a:\n    environment:\n      A: ${\xff}\n      \xc3: $\xe2\n    env_file: [\"\xff\"]\n"))
	write("bomb.compose.yml", []byte("a: &a [x,x,x,x,x,x,x,x,x]\nb: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]\nc: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]\nd: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]\nservices: {s: {environment: *d}}\n"))
	write(".github/workflows/bad.yml", []byte("on: push\njobs:\n  j:\n    steps:\n      - run: ${{ secrets.\xff }} ${{ vars.A\xc3 }}\n"))

	// Hostile names: terminal escape sequences, newlines and bidi controls.
	write(".env.\x1b[2J\x1b]0;pwned\x07prod", []byte("ESC_ONE=1\n"))
	write("src/\x1b[31mred\u202etxt.js", []byte("process.env.ESC_VAR\n"))
	write("a\nb/.env.ci", []byte("NEWLINE=1\n"))

	// Hostile links.
	link := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Logf("cannot create symlink %s: %v", name, err)
		}
	}
	link("/etc", "etc-link")
	link("/etc/passwd", ".env.passwd")
	link("..", "up")
	link(".", "self")
	link("nowhere", ".env.broken")
	link("loop-b", "loop-a")
	link("loop-a", "loop-b")
	return dir
}

// controlBytes reports the first byte sequence in s that a terminal would
// interpret: C0 controls other than \n and \t, DEL, C1 controls and bidi
// overrides.
func controlBytes(s string) (string, bool) {
	for i, r := range s {
		switch {
		case r == '\n' || r == '\t':
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			return strconv.QuoteToASCII(s[max(0, i-20):min(len(s), i+10)]), true
		case (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) || r == 0x200e || r == 0x200f:
			return strconv.QuoteToASCII(s[max(0, i-20):min(len(s), i+10)]), true
		}
	}
	return "", false
}

// TestHostileRepositoryNeverCrashesOrInjects runs every command and format
// over the hostile repository: no panic, exit 0/1, bounded time, and no
// terminal control sequence from a file name in any text output.
func TestHostileRepositoryNeverCrashesOrInjects(t *testing.T) {
	dir := hostileRepo(t)
	textFormats := []string{"pretty", "doctor", "github"}
	invocations := [][]string{{"--verbose", "check"}, {"doctor"}, {"diff"}, {"diff", "default", "production"}, {"init", "--dry-run", "--example"}}
	// One run per output format is enough: they share the analysis.
	for _, f := range []string{"pretty", "json", "sarif", "github"} {
		invocations = append(invocations, []string{"check", "--format", f})
	}
	invocations = append(invocations, []string{"doctor", "--format", "doctor"}, []string{"check", "--process-env"})

	start := time.Now()
	for _, args := range invocations {
		name := strings.Join(args, " ")
		var stdout, stderr bytes.Buffer
		code := run(append([]string{"--root", dir}, args...), &stdout, &stderr)
		out := stdout.String() + stderr.String()
		if code != exitOK && code != exitFindings {
			t.Errorf("envguard %s: exit %d: %s", name, code, stderr.String())
		}
		if strings.Contains(out, "internal error") || strings.Contains(out, "goroutine ") || strings.Contains(out, "panic") {
			t.Errorf("envguard %s crashed: %s", name, out)
		}
		isText := !slices.Contains(args, "json") && !slices.Contains(args, "sarif")
		if isText && (len(args) > 0) {
			if seq, bad := controlBytes(out); bad {
				t.Errorf("envguard %s prints a terminal control sequence from a file name: %s", name, seq)
			}
		}
	}
	_ = textFormats
	// A few seconds normally, well under a minute under -race on a small CI
	// runner. The limit only has to catch a stall or quadratic output, which
	// took many minutes before they were fixed, so it is generous.
	if d := time.Since(start); d > 5*time.Minute {
		t.Errorf("hostile repository took %s", d)
	}
	// The other platforms' symlink semantics differ; just make sure the
	// escape links were not followed on unix.
	if runtime.GOOS != "windows" {
		var stdout, stderr bytes.Buffer
		run([]string{"--root", dir, "check", "--format", "json", "--fail-on", "never"}, &stdout, &stderr)
		for _, leak := range []string{"root:", "/bin/sh", "daemon"} { // /etc/passwd content
			if strings.Contains(stdout.String(), leak) {
				t.Errorf("content of a file outside the root appears in the output (%q)", leak)
			}
		}
	}
}

var _ = io.Discard

// TestWriteHostileRepo is a developer tool, skipped unless ENVGUARD_WRITE_HOSTILE
// names an empty directory: it writes the hostile repository there so the
// binary can be timed or profiled on it by hand.
func TestWriteHostileRepo(t *testing.T) {
	dest := os.Getenv("ENVGUARD_WRITE_HOSTILE")
	if dest == "" {
		t.Skip("set ENVGUARD_WRITE_HOSTILE=<dir> to write the hostile repository")
	}
	src := hostileRepo(t)
	if err := os.CopyFS(dest, os.DirFS(src)); err != nil {
		t.Logf("copy: %v (symlinks may not copy)", err)
	}
}
