package testutil

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// FakeSecret returns a deterministic, secret-shaped value of the given kind,
// assembled at runtime so that no secret-looking literal exists anywhere in
// the repository (GitHub push protection and secret scanners stay quiet).
// seed makes values unique; marker, if non-empty, is embedded so tests can
// search outputs for it. Kinds: aws, github, slack, stripe, openai,
// anthropic, google, sendgrid, twilio, pem, jwt, url, entropy.
func FakeSecret(kind string, seed int, marker string) string {
	alnum := func(n int) string {
		const chars = "Q7wR2tY9uP4aS6dF8gH3jK5lZ1xC0vBnM"
		var b strings.Builder
		b.WriteString(marker)
		for i := 0; b.Len() < n; i++ {
			b.WriteByte(chars[(seed*31+i*17)%len(chars)])
		}
		return b.String()
	}
	upper := func(n int) string { return strings.ToUpper(alnum(n)) }
	hex := func(n int) string {
		var b strings.Builder
		for i := range n {
			b.WriteByte("0123456789abcdef"[(seed*7+i*13)%16])
		}
		return b.String()
	}
	switch kind {
	case "aws":
		return "AK" + "IA" + upper(16)[:16]
	case "github":
		return "gh" + "p_" + alnum(36)
	case "slack":
		return "xo" + "xb-" + "1029384756-" + alnum(24)
	case "stripe":
		return "sk" + "_li" + "ve_" + alnum(28)
	case "openai":
		return "sk" + "-pr" + "oj-" + alnum(40)
	case "anthropic":
		return "sk" + "-an" + "t-api03-" + alnum(48)
	case "google":
		return "AI" + "za" + alnum(35)[:35]
	case "sendgrid":
		return "S" + "G." + alnum(22)[:22] + "." + alnum(43)[:43]
	case "twilio":
		return "S" + "K" + hex(32)
	case "pem":
		return "-----BEGIN " + "RSA PRI" + "VATE KEY-----" + alnum(40)
	case "jwt":
		return "ey" + "JhbGciOiJIUzI1NiJ9." + "ey" + "J" + alnum(20) + "." + alnum(30)
	case "url":
		return "postgres://app:" + alnum(18) + "@db.internal.corp:5432/app"
	case "entropy":
		return alnum(32)
	default:
		panic("testutil: unknown fake secret kind " + kind)
	}
}

// SecretKinds lists every kind FakeSecret supports.
var SecretKinds = []string{"aws", "github", "slack", "stripe", "openai", "anthropic", "google", "sendgrid",
	"twilio", "pem", "jwt", "url", "entropy"}

var secretMarkerRE = regexp.MustCompile(`\{\{secret:([a-z]+)\}\}`)

var placeholderMarkerRE = regexp.MustCompile(`\{\{placeholder:([a-z]+)\}\}`)

// Placeholder returns a value that has the *shape* of a provider's secret but
// is an obvious placeholder, which EnvGuard must not flag (negative fixtures).
// Like FakeSecret it is built at run time, never written literally into a
// committed file: GitHub push protection (rightly) refuses a commit that
// contains something shaped like a Stripe key, even a placeholder.
func Placeholder(kind string) string {
	switch kind {
	case "stripe":
		return "sk_" + "live_" + strings.Repeat("x", 24)
	default:
		panic("testutil: unknown placeholder kind " + kind)
	}
}

// CopyFixture copies a fixture tree to a temporary directory and returns
// it. While copying it:
//   - renames files called _gitignore to .gitignore (a real nested .gitignore
//     would make git drop the fixture files it ignores);
//   - replaces {{secret:KIND}} markers with FakeSecret values and
//     {{placeholder:KIND}} markers with Placeholder values;
//   - if the tree has a git-tracked.txt manifest, initializes a git
//     repository in the copy and `git add -f`s the listed paths (skipping the
//     test if git is not installed).
//
// Tests that depend on git state must run on such a copy, never on the
// working tree of this repository.
func CopyFixture(t testing.TB, src string) string {
	t.Helper()
	dst := t.TempDir()
	seed := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.Name() == "_gitignore" {
			rel = filepath.Join(filepath.Dir(rel), ".gitignore")
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(p) //nolint:gosec // test fixture
		if err != nil {
			return err
		}
		data = secretMarkerRE.ReplaceAllFunc(data, func(m []byte) []byte {
			seed++
			return []byte(FakeSecret(string(secretMarkerRE.FindSubmatch(m)[1]), seed, ""))
		})
		data = placeholderMarkerRE.ReplaceAllFunc(data, func(m []byte) []byte {
			return []byte(Placeholder(string(placeholderMarkerRE.FindSubmatch(m)[1])))
		})
		return os.WriteFile(target, data, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(dst, "git-tracked.txt")) //nolint:gosec // test fixture
	if err == nil {
		var tracked []string
		for _, line := range strings.Split(string(manifest), "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				tracked = append(tracked, line)
			}
		}
		InitGit(t, dst, tracked)
	}
	return dst
}

// InitGit creates a git repository in dir and stages the given paths (with
// -f, so ignored files can be tracked). It skips the test if git is missing.
func InitGit(t testing.TB, dir string, tracked []string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // test helper
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if len(tracked) > 0 {
		run(append([]string{"add", "-f", "--"}, tracked...)...)
	}
}

// canaryKinds are the FakeSecret kinds that can carry a text marker
// (Twilio keys are hex-only).
var canaryKinds = []string{"aws", "github", "slack", "stripe", "openai", "anthropic", "google", "sendgrid",
	"pem", "jwt", "url", "entropy"}

// Canary returns a unique, secret-shaped value for leak tests. Every value
// contains the marker "CNRY" followed by the zero-padded index, so outputs
// can be searched for "CNRY" to detect any leak.
func Canary(i int) string {
	return FakeSecret(CanaryKind(i), i, fmt.Sprintf("CNRY%04d", i))
}

// CanaryKind is the secret kind Canary(i) uses.
func CanaryKind(i int) string { return canaryKinds[i%len(canaryKinds)] }
