package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "init.defaultBranch=main"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectRepo(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, ".gitignore", ".env\n*.local\n")
	write(t, dir, ".env", "A=1\n")
	write(t, dir, ".env.local", "A=2\n")
	write(t, dir, ".env.production", "A=3\n")
	write(t, dir, "apps/api/.env", "A=4\n")
	write(t, dir, "with space/.env.test", "A=5\n")
	write(t, dir, ".env.example", "A=\n")
	git(t, dir, "add", ".gitignore", ".env.example", ".env.production", "with space/.env.test")
	git(t, dir, "add", "-f", "apps/api/.env") // tracked even though ignored

	queried := []string{".env", ".env.local", ".env.production", "apps/api/.env", ".env.example", "with space/.env.test", "missing/.env"}
	st := Inspect(context.Background(), dir, queried)
	if !st.Available {
		t.Fatalf("unavailable: %s", st.Reason)
	}
	for p, want := range map[string]bool{".env.production": true, "apps/api/.env": true, "with space/.env.test": true, ".env": false, ".env.example": true} {
		if st.Tracked[p] != want {
			t.Errorf("Tracked[%s] = %v, want %v", p, st.Tracked[p], want)
		}
	}
	for p, want := range map[string]bool{".env": true, ".env.local": true, ".env.production": false, "apps/api/.env": false, "missing/.env": true} {
		if st.Ignored[p] != want {
			t.Errorf("Ignored[%s] = %v, want %v", p, st.Ignored[p], want)
		}
	}
}

func TestInspectSubdirectoryRoot(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "svc/.env.production", "A=1\n")
	git(t, dir, "add", "svc/.env.production")
	st := Inspect(context.Background(), filepath.Join(dir, "svc"), []string{".env.production"})
	if !st.Available || !st.Tracked[".env.production"] {
		t.Errorf("paths must be relative to the root: %+v", st)
	}
}

func TestInspectNothingIgnored(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	st := Inspect(context.Background(), dir, []string{".env"})
	if !st.Available || len(st.Ignored) != 0 {
		t.Errorf("exit code 1 of check-ignore must not be an error: %+v", st)
	}
}

func TestInspectNotARepo(t *testing.T) {
	gitAvailable(t)
	st := Inspect(context.Background(), t.TempDir(), []string{".env"})
	if st.Available || st.Reason == "" {
		t.Errorf("want unavailable with a reason, got %+v", st)
	}
}

func TestInspectNoGit(t *testing.T) {
	st := inspect(context.Background(), "envguard-no-such-git-binary", t.TempDir(), nil)
	if st.Available || st.Reason != "git is not installed" {
		t.Errorf("got %+v", st)
	}
}

func TestInspectIgnoresMaliciousFsmonitor(t *testing.T) {
	gitAvailable(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	marker := filepath.Join(t.TempDir(), "pwned")
	// A repository config that would run a program on index refresh.
	git(t, dir, "config", "core.fsmonitor", "touch "+marker)
	write(t, dir, "a.txt", "x")
	git(t, dir, "-c", "core.fsmonitor=false", "add", "a.txt")
	_ = Inspect(context.Background(), dir, []string{"a.txt"})
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("core.fsmonitor from the repository was executed")
	}
}

// File and directory names reach git as stdin (NUL-separated) and in git's
// -z output, never through a shell or as option-looking arguments. Names with
// spaces, Unicode, quotes, leading dashes, tabs and newlines must work, also
// when the repository root itself has such a name.
func TestInspectUnusualNames(t *testing.T) {
	gitAvailable(t)
	names := []string{
		"plain", "with space", "ünï cødé 日本", "-dash", "--help", "--git-dir=x", "-C", "a'quote", "a\"dq", "semi;colon",
		"$(touch pwned)", "`touch pwned`", "tab\there", "new\nline", "back\\slash", "percent%20", "star*glob", "[bracket]",
	}
	root := filepath.Join(t.TempDir(), "-root dir ü")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q")
	write(t, root, ".gitignore", "*.env\n")
	var wantTracked, wantIgnored, queried []string
	for _, n := range names {
		if runtime.GOOS == "windows" && strings.Contains(n, `\`) {
			continue // a backslash is a path separator there: a different directory layout
		}
		tracked := n + "/.env.example"
		ignored := n + "/real.env"
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Logf("cannot create %q here: %v", n, err)
			continue
		}
		write(t, root, tracked, "A=\n")
		write(t, root, ignored, "A=1\n")
		git(t, root, "add", "--", tracked)
		wantTracked = append(wantTracked, tracked)
		wantIgnored = append(wantIgnored, ignored)
		queried = append(queried, ignored, tracked)
	}
	if len(wantTracked) < 6 {
		t.Fatalf("only %d directories could be created", len(wantTracked))
	}
	st := Inspect(context.Background(), root, queried)
	if !st.Available {
		t.Fatalf("git unavailable: %s", st.Reason)
	}
	for _, p := range wantTracked {
		if !st.Tracked[p] {
			t.Errorf("tracked file %q not reported as tracked", p)
		}
		if st.Ignored[p] {
			t.Errorf("tracked file %q reported as ignored", p)
		}
	}
	for _, p := range wantIgnored {
		if !st.Ignored[p] {
			t.Errorf("ignored file %q not reported as ignored", p)
		}
		if st.Tracked[p] {
			t.Errorf("untracked file %q reported as tracked", p)
		}
	}
	// No name was ever interpreted as a command or an option.
	for _, probe := range []string{"pwned", "x"} {
		if _, err := os.Stat(filepath.Join(root, probe)); err == nil {
			t.Errorf("%s was created: a name was executed", probe)
		}
	}
}

// A root that is not under git, does not exist, or is a file never fails.
func TestInspectNeverFailsOnOddRoots(t *testing.T) {
	gitAvailable(t)
	file := filepath.Join(t.TempDir(), "file")
	write(t, filepath.Dir(file), "file", "x")
	for _, root := range []string{file, filepath.Join(t.TempDir(), "missing"), "", "-", "--version"} {
		if st := Inspect(context.Background(), root, []string{"a"}); st.Available && root != "" {
			t.Errorf("Inspect(%q) = available", root)
		}
	}
}
