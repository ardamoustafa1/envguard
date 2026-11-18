package discovery

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/testutil"
)

// copyFixture copies testdata/discovery/<name> into a temp dir. Files named
// _gitignore become .gitignore: a real nested .gitignore in this repository
// would make git ignore the very fixture files it is meant to ignore.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", "discovery", name)
	dst := t.TempDir()
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
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func discover(t *testing.T, opts Options) *Inventory {
	t.Helper()
	inv, err := Discover(opts)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return inv
}

func TestGitExcludeCannotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "info/exclude", "app.js\n")
	write(t, root, "app.js", "x")
	symlink(t, outside, filepath.Join(root, ".git"))
	inv := discover(t, Options{Root: root})
	if !slices.Contains(paths(inv.Files), "app.js") {
		t.Error("an external .git/info/exclude changed repository discovery")
	}
}

// goldenView is the inventory without the machine-specific Root.
type goldenView struct {
	Files     []File
	Skipped   []Skipped
	AppRoots  []string
	Truncated bool
}

func TestGolden(t *testing.T) {
	for _, name := range []string{"monorepo", "nested-gitignore", "env-variants", "binary"} {
		t.Run(name, func(t *testing.T) {
			inv := discover(t, Options{Root: copyFixture(t, name)})
			got, err := json.MarshalIndent(goldenView{inv.Files, inv.Skipped, inv.AppRoots, inv.Truncated}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			testutil.Golden(t, filepath.Join("testdata", "discovery", name+".golden.json"), append(got, '\n'))
		})
	}
}

func paths(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func find(inv *Inventory, p string) (File, bool) {
	for _, f := range inv.Files {
		if f.Path == p {
			return f, true
		}
	}
	return File{}, false
}

func skipReason(inv *Inventory, p string) SkipReason {
	for _, s := range inv.Skipped {
		if s.Path == p {
			return s.Reason
		}
	}
	return ""
}

func TestMonorepo(t *testing.T) {
	inv := discover(t, Options{Root: copyFixture(t, "monorepo")})

	wantApp := map[string]string{
		".env.example":                     ".",
		"apps/api/.env":                    "apps/api",
		"apps/api/config/.env.test":        "apps/api",
		"apps/api/src/server.ts":           "apps/api",
		"apps/web/.env.local":              "apps/web",
		"apps/web/.env.production.example": "apps/web",
		"services/worker/.env":             "services/worker",
		"services/billing/Program.cs":      "services/billing",
		"tools/deploy.sh":                  ".",
		"deploy/compose.prod.yaml":         ".",
	}
	for p, app := range wantApp {
		f, ok := find(inv, p)
		if !ok {
			t.Errorf("%s not discovered", p)
			continue
		}
		if f.AppRoot != app {
			t.Errorf("%s AppRoot = %q, want %q", p, f.AppRoot, app)
		}
	}
	if want := []string{".", "apps/api", "apps/web", "services/billing", "services/worker"}; !slices.Equal(inv.AppRoots, want) {
		t.Errorf("AppRoots = %v, want %v", inv.AppRoots, want)
	}

	if got, want := paths(inv.ByKind(KindCompose)), []string{"deploy/compose.prod.yaml", "deploy/docker-compose-dev.yml", "docker-compose.yml"}; !slices.Equal(got, want) {
		t.Errorf("compose = %v, want %v", got, want)
	}
	if got, want := paths(inv.ByKind(KindWorkflow)), []string{".github/workflows/ci.yml", ".github/workflows/release.yaml"}; !slices.Equal(got, want) {
		t.Errorf("workflows = %v, want %v", got, want)
	}
	for _, d := range []string{"vendor", "dist", "build", "target", ".venv", "venv", "__pycache__", ".nuxt", "coverage", "apps/web/node_modules", "apps/web/.next"} {
		if r := skipReason(inv, d); r != SkipBuiltinExclude {
			t.Errorf("%s skip reason = %q, want builtin-exclude", d, r)
		}
	}
	for _, f := range inv.Files {
		if strings.Contains(f.Path, "\\") || strings.HasPrefix(f.Path, "/") {
			t.Errorf("path %q is not relative and slash-separated", f.Path)
		}
	}
	if f, _ := find(inv, "apps/web/components/Button.vue"); f.Language != LangVue {
		t.Errorf("Button.vue language = %q", f.Language)
	}
}

func TestNestedGitignore(t *testing.T) {
	inv := discover(t, Options{Root: copyFixture(t, "nested-gitignore")})

	got := paths(inv.Files)
	want := []string{".env", ".env.example", "app.js", "other/c.ts", "sub/generated.js", "sub/keep.ts"}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v\nwant    %v", got, want)
	}
	if f, _ := find(inv, ".env"); !f.GitIgnored {
		t.Error(".env should be discovered and marked GitIgnored")
	}
	if f, _ := find(inv, ".env.example"); f.GitIgnored {
		t.Error(".env.example should not be GitIgnored")
	}
	for p, r := range map[string]SkipReason{
		"secret":          SkipGitIgnored, // ignored directory is pruned
		"generated.js":    SkipGitIgnored, // root-anchored pattern
		"sub/a.ts":        SkipGitIgnored, // nested .gitignore with CRLF endings
		"sub/deeper/b.ts": SkipGitIgnored, // nested pattern applies below its dir
	} {
		if got := skipReason(inv, p); got != r {
			t.Errorf("%s skip = %q, want %q", p, got, r)
		}
	}
}

func TestGitInfoExclude(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".git/info/exclude", "local-only.js\n")
	write(t, root, "local-only.js", "x")
	write(t, root, "kept.js", "x")
	inv := discover(t, Options{Root: root})
	if got := paths(inv.Files); !slices.Equal(got, []string{"kept.js"}) {
		t.Errorf("files = %v", got)
	}
}

func TestEnvVariants(t *testing.T) {
	inv := discover(t, Options{Root: copyFixture(t, "env-variants")})
	type want struct {
		kind  Kind
		env   string
		local bool
	}
	wants := map[string]want{
		".env":                    {KindEnv, "default", false},
		".env.local":              {KindEnv, "development", true},
		".env.dev":                {KindEnv, "development", false},
		".env.development":        {KindEnv, "development", false},
		".env.development.local":  {KindEnv, "development", true},
		".env.test":               {KindEnv, "test", false},
		".env.testing":            {KindEnv, "test", false},
		".env.stage":              {KindEnv, "staging", false},
		".env.staging":            {KindEnv, "staging", false},
		".env.prod":               {KindEnv, "production", false},
		".env.production":         {KindEnv, "production", false},
		".env.production.local":   {KindEnv, "production", true},
		".env.preview":            {KindEnv, "preview", false},
		".env.vault":              {KindEnv, "vault", false},
		".env.example":            {KindEnvTemplate, "default", false},
		".env.sample":             {KindEnvTemplate, "default", false},
		".env.template":           {KindEnvTemplate, "default", false},
		".env.dist":               {KindEnvTemplate, "default", false},
		".env.defaults":           {KindEnvTemplate, "default", false},
		".env.production.example": {KindEnvTemplate, "production", false},
		".env.staging.sample":     {KindEnvTemplate, "staging", false},
		".env.test.template":      {KindEnvTemplate, "test", false},
		".env.local.example":      {KindEnvTemplate, "development", true},
		".env.prod.local.example": {KindEnvTemplate, "production", true},
	}
	if len(inv.Files) != len(wants) {
		t.Errorf("got %d files %v, want %d", len(inv.Files), paths(inv.Files), len(wants))
	}
	for p, w := range wants {
		f, ok := find(inv, p)
		if !ok {
			t.Errorf("%s not discovered", p)
			continue
		}
		if f.Kind != w.kind || f.Environment != w.env || f.Local != w.local {
			t.Errorf("%s = %s/%s/local=%v, want %s/%s/local=%v", p, f.Kind, f.Environment, f.Local, w.kind, w.env, w.local)
		}
	}
}

func TestClassifyEnvName(t *testing.T) {
	tests := []struct {
		name  string
		kind  Kind
		env   string
		local bool
		ok    bool
	}{
		{".env", KindEnv, "default", false, true},
		{".env.PROD", KindEnv, "production", false, true},
		{".env.Staging.Example", KindEnv, "staging.example", false, true}, // suffixes are case-sensitive
		{".env.", "", "", false, false},
		{".env..x", "", "", false, false},
		{".envrc", "", "", false, false},
		{".environment", "", "", false, false},
		{"env.example", "", "", false, false},
		{"app.env", "", "", false, false},
		{".env.eu.prod", KindEnv, "eu.prod", false, true},
	}
	for _, tt := range tests {
		kind, env, local, ok := ClassifyEnvName(tt.name)
		if kind != tt.kind || env != tt.env || local != tt.local || ok != tt.ok {
			t.Errorf("ClassifyEnvName(%q) = %q,%q,%v,%v; want %q,%q,%v,%v", tt.name, kind, env, local, ok, tt.kind, tt.env, tt.local, tt.ok)
		}
	}
}

func TestComposeNames(t *testing.T) {
	for stem, want := range map[string]bool{
		"compose": true, "compose.override": true, "compose-prod": true, "docker-compose": true,
		"docker-compose.prod": true, "docker-compose_dev": true, "composer": false, "my-compose": false, "docker": false,
	} {
		if got := isComposeName(stem); got != want {
			t.Errorf("isComposeName(%q) = %v", stem, got)
		}
	}
}

func TestBinary(t *testing.T) {
	inv := discover(t, Options{Root: copyFixture(t, "binary")})
	if got := paths(inv.Files); !slices.Equal(got, []string{"text.js"}) {
		t.Errorf("files = %v", got)
	}
	for _, p := range []string{"blob.js", ".env.binary"} {
		if r := skipReason(inv, p); r != SkipBinary {
			t.Errorf("%s skip = %q, want binary", p, r)
		}
	}
	if r := skipReason(inv, "image.png"); r != "" {
		t.Errorf("unclassified file recorded as skipped: %q", r)
	}
}

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHugeFiles(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("a", DefaultMaxSourceSize+1)
	write(t, root, "big.js", big)
	write(t, root, ".env", "K="+big)
	write(t, root, "docker-compose.yml", "#"+big)
	write(t, root, "small.js", "x")
	inv := discover(t, Options{Root: root})
	if got := paths(inv.Files); !slices.Equal(got, []string{".env", "small.js"}) {
		t.Errorf("files = %v (env files are kept so the parser can report them)", got)
	}
	for _, p := range []string{"big.js", "docker-compose.yml"} {
		if r := skipReason(inv, p); r != SkipTooLarge {
			t.Errorf("%s skip = %q, want too-large", p, r)
		}
	}
}

func TestLimits(t *testing.T) {
	t.Run("max depth", func(t *testing.T) {
		root := t.TempDir()
		deep := strings.Repeat("d/", 30)
		write(t, root, deep+"deep.js", "x")
		write(t, root, strings.Repeat("d/", 25)+"ok.js", "x")
		inv := discover(t, Options{Root: root})
		if got := paths(inv.Files); len(got) != 1 || !strings.HasSuffix(got[0], "/ok.js") {
			t.Errorf("files = %v", got)
		}
		if r := skipReason(inv, strings.Repeat("d/", 26)[:26*2-1]); r != SkipMaxDepth {
			t.Errorf("max-depth not recorded; skipped = %v", inv.Skipped)
		}
	})
	t.Run("max files", func(t *testing.T) {
		root := t.TempDir()
		for i := range 20 {
			write(t, root, filepath.Join("src", strings.Repeat("a", i+1)+".js"), "x")
		}
		inv := discover(t, Options{Root: root, MaxFiles: 10})
		if !inv.Truncated || len(inv.Files) != 10 {
			t.Errorf("truncated=%v files=%d", inv.Truncated, len(inv.Files))
		}
		n := 0
		for _, s := range inv.Skipped {
			if s.Reason == SkipMaxFiles {
				n++
			}
		}
		if n != 1 {
			t.Errorf("max-files recorded %d times", n)
		}
	})
}

func TestOptions(t *testing.T) {
	root := copyFixture(t, "monorepo")
	t.Run("exclude", func(t *testing.T) {
		inv := discover(t, Options{Root: root, Exclude: []string{"apps/web/**", "*.sh", "services"}})
		for _, f := range inv.Files {
			if strings.HasPrefix(f.Path, "apps/web/") || strings.HasPrefix(f.Path, "services/") || strings.HasSuffix(f.Path, ".sh") {
				t.Errorf("%s should be excluded", f.Path)
			}
		}
		if r := skipReason(inv, "apps/web"); r != SkipUserExclude {
			t.Errorf("apps/web skip = %q", r)
		}
	})
	t.Run("include", func(t *testing.T) {
		inv := discover(t, Options{Root: root, Include: []string{"apps/api/**"}})
		for _, f := range inv.Files {
			if !strings.HasPrefix(f.Path, "apps/api/") {
				t.Errorf("%s should not be included", f.Path)
			}
		}
		if len(inv.Files) == 0 {
			t.Error("no files included")
		}
	})
	t.Run("languages", func(t *testing.T) {
		inv := discover(t, Options{Root: root, Languages: []Language{LangGo}})
		for _, f := range inv.ByKind(KindSource) {
			if f.Language != LangGo {
				t.Errorf("%s (%s) should be filtered", f.Path, f.Language)
			}
		}
		if len(inv.ByKind(KindEnv)) == 0 {
			t.Error("language filter must not drop env files")
		}
	})
	t.Run("environment overrides", func(t *testing.T) {
		r := t.TempDir()
		write(t, r, "deploy/prod.env", "K=1")
		write(t, r, "deploy/prod.env.example", "K=")
		write(t, r, ".env.production", "K=1")
		inv := discover(t, Options{Root: r, Environments: []EnvironmentPatterns{
			{Name: "live", Files: []string{"deploy/prod.env", ".env.production"}, Templates: []string{"deploy/*.example"}},
		}})
		want := map[string]File{
			"deploy/prod.env":         {Kind: KindEnv, Environment: "live"},
			"deploy/prod.env.example": {Kind: KindEnvTemplate, Environment: "live"},
			".env.production":         {Kind: KindEnv, Environment: "live"},
		}
		for p, w := range want {
			f, ok := find(inv, p)
			if !ok || f.Kind != w.Kind || f.Environment != w.Environment {
				t.Errorf("%s = %+v, want %s/%s", p, f, w.Kind, w.Environment)
			}
		}
	})
	t.Run("invalid pattern", func(t *testing.T) {
		if _, err := Discover(Options{Root: root, Exclude: []string{"a["}}); err == nil {
			t.Error("want error")
		}
	})
}

func TestRootErrors(t *testing.T) {
	if _, err := Discover(Options{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("missing root: want error")
	}
	f := write(t, t.TempDir(), "file", "x")
	if _, err := Discover(Options{Root: f}); err == nil {
		t.Error("file root: want error")
	}
}

func TestDeterministic(t *testing.T) {
	root := copyFixture(t, "monorepo")
	a := discover(t, Options{Root: root})
	b := discover(t, Options{Root: root})
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if !bytes.Equal(ja, jb) {
		t.Error("discovery is not deterministic")
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks not supported here: %v", err)
		}
		t.Fatal(err)
	}
}

func TestSymlinks(t *testing.T) {
	outside := t.TempDir()
	write(t, outside, ".env", "SECRET=1")
	write(t, outside, "lib/x.js", "x")

	root := t.TempDir()
	write(t, root, ".env.development", "A=1")
	write(t, root, "src/app.js", "x")
	symlink(t, filepath.Join(outside, ".env"), filepath.Join(root, "escape.env.link", ".env"))
	symlink(t, filepath.Join(outside, "lib"), filepath.Join(root, "outside-dir"))
	symlink(t, ".env.development", filepath.Join(root, ".env"))              // file inside root: followed
	symlink(t, "..", filepath.Join(root, "src", "parent-loop"))              // points to an ancestor
	symlink(t, ".", filepath.Join(root, "src", "self-loop"))                 // points to its own dir
	symlink(t, filepath.Join(root, "src"), filepath.Join(root, "src-alias")) // dir inside root: duplicate
	symlink(t, "missing.js", filepath.Join(root, "broken.js"))
	symlink(t, "b.js", filepath.Join(root, "a.js")) // a.js -> b.js -> a.js
	symlink(t, "a.js", filepath.Join(root, "b.js"))

	inv := discover(t, Options{Root: root})

	if got := paths(inv.Files); !slices.Equal(got, []string{".env", ".env.development", "src/app.js"}) {
		t.Errorf("files = %v", got)
	}
	if f, _ := find(inv, ".env"); f.Kind != KindEnv || f.Environment != "default" {
		t.Errorf(".env via symlink = %+v", f)
	}
	for p, r := range map[string]SkipReason{
		"escape.env.link/.env": SkipSymlinkOutside,
		"outside-dir":          SkipSymlinkOutside,
		"src/parent-loop":      SkipSymlinkLoop,
		"src/self-loop":        SkipSymlinkLoop,
		"src-alias":            SkipSymlinkSeen,
		"broken.js":            SkipSymlinkBroken,
		"a.js":                 SkipSymlinkLoop,
		"b.js":                 SkipSymlinkLoop,
	} {
		if got := skipReason(inv, p); got != r {
			t.Errorf("%s skip = %q, want %q", p, got, r)
		}
	}
}

// TestSymlinkedRoot checks that a root reached through a symlink works and
// that links into the real root are not considered "outside".
func TestSymlinkedRoot(t *testing.T) {
	realDir := t.TempDir()
	write(t, realDir, ".env", "A=1")
	write(t, realDir, "conf/.env.test", "A=2")
	symlink(t, filepath.Join(realDir, "conf", ".env.test"), filepath.Join(realDir, ".env.test"))
	link := filepath.Join(t.TempDir(), "root-link")
	symlink(t, realDir, link)

	inv := discover(t, Options{Root: link})
	if got := paths(inv.Files); !slices.Equal(got, []string{".env", ".env.test", "conf/.env.test"}) {
		t.Errorf("files = %v; skipped = %v", got, inv.Skipped)
	}
}
