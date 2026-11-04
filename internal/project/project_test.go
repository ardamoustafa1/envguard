package project

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
)

func build(t *testing.T, files map[string]string, opts Options) *Project {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := config.Load(config.Flags{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := discovery.Discover(s.DiscoveryOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Build(s, inv, opts)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildGroups(t *testing.T) {
	p := build(t, map[string]string{
		"package.json":            "{}",
		".env":                    "A=1\n",
		".env.example":            "A=\n",
		".env.local.example":      "L=\n",
		".env.staging.example":    "S=\n",
		".env.production":         "A=2\n",
		".env.production.example": "P=\n",
		"apps/api/package.json":   "{}",
		"apps/api/.env":           "X=1\n",
		"apps/api/src/server.ts":  "x",
	}, Options{})

	var roots []string
	for _, a := range p.Apps {
		roots = append(roots, a.Root)
	}
	if !slices.Equal(roots, []string{".", "apps/api"}) {
		t.Errorf("apps = %v", roots)
	}
	root := p.AppFor(".")
	var envs []string
	for _, g := range root.Groups {
		envs = append(envs, g.Environment)
	}
	if !slices.Equal(envs, []string{"default", "development", "production", "staging"}) {
		t.Errorf("groups = %v", envs)
	}
	var tpl []string
	for _, f := range root.ApplicableTemplates("production", false) {
		tpl = append(tpl, f.Path)
	}
	if !slices.Equal(tpl, []string{".env.example", ".env.production.example"}) {
		t.Errorf("applicable = %v", tpl)
	}
	if got := len(root.ApplicableTemplates("development", false)); got != 1 {
		t.Errorf("local template must not apply to non-local files, got %d templates", got)
	}
	if len(p.EnvFiles) != 7 {
		t.Errorf("EnvFiles = %d", len(p.EnvFiles))
	}
	if f := p.File(".env"); f == nil || f.Parsed == nil || !slices.Equal(f.Keys(), []string{"A"}) {
		t.Errorf("File(.env) = %+v", f)
	}
	if anc := p.AncestorApps("apps/api"); len(anc) != 1 || anc[0].Root != "." {
		t.Errorf("AncestorApps = %v", anc)
	}
	if p.ComposeRefs != nil || p.Schema != nil || p.Git == nil || p.Git.Available {
		t.Error("later-stage slots should be empty")
	}
}

func TestEnvSelected(t *testing.T) {
	p := build(t, map[string]string{".env": "A=1\n"}, Options{Environments: []string{"prod", "Staging"}})
	for env, want := range map[string]bool{"production": true, "staging": true, "default": false, "development": false} {
		if got := p.EnvSelected(env); got != want {
			t.Errorf("EnvSelected(%s) = %v", env, got)
		}
	}
	all := build(t, map[string]string{".env": "A=1\n"}, Options{})
	if !all.EnvSelected("anything") {
		t.Error("no filter must select everything")
	}
}

func TestInlineIgnores(t *testing.T) {
	p := build(t, map[string]string{
		".env": "# envguard-ignore-file EG002\n# envguard-ignore EG006\nA=1\nB=2\n",
	}, Options{})
	if !p.IsIgnored("EG002", ".env", 0) || !p.IsIgnored("EG006", ".env", 3) || p.IsIgnored("EG006", ".env", 4) || p.IsIgnored("EG006", "nope", 3) {
		t.Error("IsIgnored wrong")
	}
}

func TestUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := config.Load(config.Flags{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := discovery.Discover(s.DiscoveryOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	p, err := Build(s, inv, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if f := p.File(".env"); f == nil || f.Parsed != nil || f.ReadErr == "" {
		t.Errorf("unreadable file = %+v", f)
	}
}
