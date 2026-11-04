package discovery

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// builtinExcludes are directory names that are never walked.
var builtinExcludes = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, ".venv": true, "venv": true, "__pycache__": true, ".next": true,
	".nuxt": true, "coverage": true,
}

// appMarkers are file names that make their directory an app root.
// Dockerfiles and Compose files are deliberately not markers: they often live
// in a deploy/ or docker/ directory away from the code they configure.
var appMarkers = map[string]bool{
	"package.json": true, "go.mod": true, "pyproject.toml": true, "requirements.txt": true,
	"setup.py": true, "Pipfile": true, "Gemfile": true, "composer.json": true, "Cargo.toml": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true, "mix.exs": true,
	"deno.json": true, "deno.jsonc": true,
}

// templateSuffixes mark an env file as a template (.env.example, .env.prod.sample, ...).
var templateSuffixes = map[string]bool{
	"example": true, "sample": true, "template": true, "dist": true, "defaults": true,
}

// Discover walks opts.Root and returns a classified, sorted inventory. It
// never follows symlinks out of the root, never opens non-regular files and
// never fails because of an individual unreadable file or directory; those
// are recorded in Inventory.Skipped. The error is reserved for an unusable
// root or invalid options.
func Discover(opts Options) (*Inventory, error) {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = DefaultMaxDepth
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = DefaultMaxFiles
	}
	if opts.MaxSourceSize <= 0 {
		opts.MaxSourceSize = DefaultMaxSourceSize
	}
	if opts.Root == "" {
		opts.Root = "."
	}
	for _, p := range slices.Concat(opts.Include, opts.Exclude) {
		if !doublestar.ValidatePattern(p) {
			return nil, fmt.Errorf("invalid glob pattern %q", p)
		}
	}

	abs, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		return nil, fmt.Errorf("root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %s is not a directory", opts.Root)
	}

	w := &walker{opts: opts, root: realRoot, inv: &Inventory{Root: realRoot}}
	if len(opts.Languages) > 0 {
		w.langs = map[Language]bool{}
		for _, l := range opts.Languages {
			w.langs[l] = true
		}
	}
	if err := w.walkDir(realRoot, "", 0, w.rootPatterns(), "."); err != nil {
		return nil, err
	}

	inv := w.inv
	slices.SortFunc(inv.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(inv.Skipped, func(a, b Skipped) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(string(a.Reason), string(b.Reason))
	})
	seen := map[string]bool{}
	for _, f := range inv.Files {
		if !seen[f.AppRoot] {
			seen[f.AppRoot] = true
			inv.AppRoots = append(inv.AppRoots, f.AppRoot)
		}
	}
	slices.Sort(inv.AppRoots)
	return inv, nil
}

type walker struct {
	opts  Options
	root  string // absolute, symlink-free
	inv   *Inventory
	langs map[Language]bool // nil = all
	files int
	stop  bool
}

func (w *walker) skip(rel string, r SkipReason) {
	w.inv.Skipped = append(w.inv.Skipped, Skipped{Path: rel, Reason: r})
}

// rootPatterns returns patterns from .git/info/exclude, which applies to the
// whole repository like a root .gitignore.
func (w *walker) rootPatterns() []gitignore.Pattern {
	return readIgnoreFile(filepath.Join(w.root, ".git", "info", "exclude"), nil)
}

// walkDir walks the real (symlink-free) directory dir, whose path relative to
// the root is rel ("" for the root).
func (w *walker) walkDir(dir, rel string, depth int, patterns []gitignore.Pattern, appRoot string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if rel == "" {
			return fmt.Errorf("read root: %w", err)
		}
		w.skip(rel, SkipUnreadable)
		return nil
	}

	var domain []string
	if rel != "" {
		domain = strings.Split(rel, "/")
	}
	for _, e := range entries {
		if e.Name() == ".gitignore" && e.Type().IsRegular() {
			own := readIgnoreFile(filepath.Join(dir, ".gitignore"), domain)
			patterns = append(slices.Clip(patterns), own...)
		}
		if e.Type().IsRegular() && isAppMarker(e.Name()) {
			appRoot = relOrDot(rel)
		}
	}
	matcher := gitignore.NewMatcher(patterns)

	for _, e := range entries {
		if w.stop {
			return nil
		}
		name := e.Name()
		childRel := path.Join(rel, name)
		childPath := filepath.Join(dir, name)
		parts := strings.Split(childRel, "/")

		typ := e.Type()
		isLink := typ&fs.ModeSymlink != 0
		var info fs.FileInfo
		if isLink {
			target, reason := w.resolveLink(childPath, dir)
			if reason != "" {
				w.skip(childRel, reason)
				continue
			}
			if info, err = os.Stat(target); err != nil {
				w.skip(childRel, SkipSymlinkBroken)
				continue
			}
			if info.IsDir() {
				// Directories inside the root are reached through their real
				// path, so following the link would only duplicate them (or
				// loop, which resolveLink already reported).
				w.skip(childRel, SkipSymlinkSeen)
				continue
			}
			childPath = target
		}

		if typ.IsDir() {
			switch {
			case builtinExcludes[name]:
				if name != ".git" {
					w.skip(childRel, SkipBuiltinExclude)
				}
			case w.excluded(childRel):
				w.skip(childRel, SkipUserExclude)
			case matcher.Match(parts, true):
				w.skip(childRel, SkipGitIgnored)
			case depth >= w.opts.MaxDepth:
				w.skip(childRel, SkipMaxDepth)
			default:
				if err := w.walkDir(childPath, childRel, depth+1, patterns, appRoot); err != nil {
					return err
				}
			}
			continue
		}

		if info == nil {
			if info, err = e.Info(); err != nil {
				continue // vanished between ReadDir and Info
			}
		}
		w.files++
		if w.files > w.opts.MaxFiles {
			w.inv.Truncated = true
			w.skip(childRel, SkipMaxFiles)
			w.stop = true
			return nil
		}
		w.file(childPath, childRel, info, matcher.Match(parts, false), appRoot)
	}
	return nil
}

// resolveLink resolves a symlink and returns its target, or a skip reason if
// it must not be followed.
func (w *walker) resolveLink(link, parentDir string) (target string, reason SkipReason) {
	t, err := filepath.EvalSymlinks(link)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) && errors.Is(err, fs.ErrNotExist) {
			return "", SkipSymlinkBroken
		}
		// Includes "too many links" for self-referencing chains.
		return "", SkipSymlinkLoop
	}
	if !within(w.root, t) {
		return "", SkipSymlinkOutside
	}
	if within(t, parentDir) {
		// The target is the directory containing the link or one of its
		// ancestors: following it would recurse forever.
		return "", SkipSymlinkLoop
	}
	return t, ""
}

// within reports whether p is base or inside base (both absolute, clean).
func within(base, p string) bool {
	r, err := filepath.Rel(base, p)
	if err != nil {
		return false
	}
	return r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

func (w *walker) file(fsPath, rel string, info fs.FileInfo, ignored bool, appRoot string) {
	f, ok := w.classify(rel)
	if !ok {
		return
	}
	if w.excluded(rel) {
		w.skip(rel, SkipUserExclude)
		return
	}
	if !w.included(rel) {
		return
	}
	isEnv := f.Kind == KindEnv || f.Kind == KindEnvTemplate
	if ignored && !isEnv {
		w.skip(rel, SkipGitIgnored)
		return
	}
	if !info.Mode().IsRegular() {
		w.skip(rel, SkipNotRegular)
		return
	}
	if !isEnv && info.Size() > w.opts.MaxSourceSize {
		w.skip(rel, SkipTooLarge)
		return
	}
	binary, err := isBinary(fsPath)
	if err != nil {
		w.skip(rel, SkipUnreadable)
		return
	}
	if binary {
		w.skip(rel, SkipBinary)
		return
	}
	f.Path = rel
	f.AppRoot = appRoot
	f.GitIgnored = ignored
	f.Size = info.Size()
	w.inv.Files = append(w.inv.Files, f)
}

// classify decides what a file is from its path alone.
func (w *walker) classify(rel string) (File, bool) {
	for _, env := range w.opts.Environments {
		if matchAny(env.Files, rel) {
			return File{Kind: KindEnv, Environment: env.Name}, true
		}
		if matchAny(env.Templates, rel) {
			return File{Kind: KindEnvTemplate, Environment: env.Name}, true
		}
	}
	base := path.Base(rel)
	if kind, env, local, ok := ClassifyEnvName(base); ok {
		return File{Kind: kind, Environment: env, Local: local}, true
	}
	ext := strings.ToLower(path.Ext(base))
	if ext == ".yml" || ext == ".yaml" {
		if path.Dir(rel) == ".github/workflows" {
			return File{Kind: KindWorkflow}, true
		}
		if isComposeName(strings.TrimSuffix(strings.ToLower(base), ext)) {
			return File{Kind: KindCompose}, true
		}
	}
	if lang, ok := LanguageForPath(base); ok && (w.langs == nil || w.langs[lang]) {
		return File{Kind: KindSource, Language: lang}, true
	}
	return File{}, false
}

// ClassifyEnvName classifies an env file by base name:
//
//	.env                     env       default
//	.env.local               env       development (local)
//	.env.<name>              env       <name>
//	.env.<name>.local        env       <name> (local)
//	.env.example             template  default   (also .sample .template .dist .defaults)
//	.env.<name>.example      template  <name>
//
// <name> is normalized with NormalizeEnvironment.
func ClassifyEnvName(base string) (kind Kind, env string, local, ok bool) {
	if base == ".env" {
		return KindEnv, EnvDefault, false, true
	}
	rest, found := strings.CutPrefix(base, ".env.")
	if !found || rest == "" {
		return "", "", false, false
	}
	parts := strings.Split(rest, ".")
	if slices.Contains(parts, "") {
		return "", "", false, false
	}
	kind = KindEnv
	if templateSuffixes[parts[len(parts)-1]] {
		kind = KindEnvTemplate
		parts = parts[:len(parts)-1]
		if len(parts) == 0 {
			return kind, EnvDefault, false, true
		}
	}
	if len(parts) > 1 && parts[len(parts)-1] == "local" {
		local = true
		parts = parts[:len(parts)-1]
	} else if len(parts) == 1 && parts[0] == "local" {
		local = true
	}
	return kind, NormalizeEnvironment(strings.Join(parts, ".")), local, true
}

// isComposeName matches docker-compose and compose, alone or followed by
// '.', '-' or '_' (compose.override, docker-compose.prod, docker-compose-dev).
func isComposeName(stem string) bool {
	for _, prefix := range []string{"docker-compose", "compose"} {
		if rest, ok := strings.CutPrefix(stem, prefix); ok {
			if rest == "" || strings.ContainsRune(".-_", rune(rest[0])) {
				return true
			}
		}
	}
	return false
}

func isAppMarker(name string) bool {
	return appMarkers[name] || strings.HasSuffix(name, ".csproj")
}

func (w *walker) excluded(rel string) bool { return matchAny(w.opts.Exclude, rel) }

func (w *walker) included(rel string) bool {
	return len(w.opts.Include) == 0 || matchAny(w.opts.Include, rel)
}

// matchAny matches rel against glob patterns. A pattern without '/' matches
// the base name at any depth; otherwise it matches the whole relative path.
func matchAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		target := rel
		if !strings.Contains(p, "/") {
			target = path.Base(rel)
		}
		if ok, err := doublestar.Match(p, target); err == nil && ok {
			return true
		}
	}
	return false
}

// readIgnoreFile parses a .gitignore-style file. Missing or unreadable files
// yield no patterns.
func readIgnoreFile(p string, domain []string) []gitignore.Pattern {
	f, err := os.Open(p) // #nosec G304 -- path is built from the walked tree
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []gitignore.Pattern
	sc := bufio.NewScanner(io.LimitReader(f, 1<<20))
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, gitignore.ParsePattern(line, domain))
	}
	return out
}

// isBinary reports whether the first 8 KiB of the file contain a NUL byte.
func isBinary(p string) (bool, error) {
	f, err := os.Open(p) // #nosec G304 -- path is built from the walked tree
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, sniffSize)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}

func relOrDot(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}
