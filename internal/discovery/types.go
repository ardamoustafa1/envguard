package discovery

import (
	"path"
	"slices"
	"strings"
)

// Limits applied while walking. Options may lower or raise them.
const (
	DefaultMaxDepth      = 25
	DefaultMaxFiles      = 50_000
	DefaultMaxSourceSize = 1 << 20 // 1 MiB
	sniffSize            = 8 << 10 // bytes inspected for NUL when detecting binary files
)

// Kind classifies a discovered file.
type Kind string

// File kinds.
const (
	KindEnv         Kind = "env"          // real env file (.env, .env.production, ...)
	KindEnvTemplate Kind = "env-template" // .env.example, .env.production.sample, ...
	KindCompose     Kind = "compose"      // docker-compose*.yml, compose*.yaml
	KindWorkflow    Kind = "workflow"     // .github/workflows/*.yml
	KindSource      Kind = "source"       // source code for the scanner
)

// Language of a source file.
type Language string

// Supported languages.
const (
	LangJavaScript Language = "javascript"
	LangTypeScript Language = "typescript"
	LangVue        Language = "vue"
	LangSvelte     Language = "svelte"
	LangPython     Language = "python"
	LangGo         Language = "go"
	LangRuby       Language = "ruby"
	LangPHP        Language = "php"
	LangRust       Language = "rust"
	LangJava       Language = "java"
	LangKotlin     Language = "kotlin"
	LangCSharp     Language = "csharp"
	LangElixir     Language = "elixir"
	LangShell      Language = "shell"
)

var extLanguages = map[string]Language{
	".js": LangJavaScript, ".jsx": LangJavaScript, ".mjs": LangJavaScript, ".cjs": LangJavaScript,
	".ts": LangTypeScript, ".tsx": LangTypeScript,
	".vue":    LangVue,
	".svelte": LangSvelte,
	".py":     LangPython,
	".go":     LangGo,
	".rb":     LangRuby,
	".php":    LangPHP,
	".rs":     LangRust,
	".java":   LangJava,
	".kt":     LangKotlin,
	".cs":     LangCSharp,
	".ex":     LangElixir, ".exs": LangElixir,
	".sh": LangShell,
}

// Languages returns all supported languages, sorted.
func Languages() []Language {
	seen := map[Language]bool{}
	var out []Language
	for _, l := range extLanguages {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return out
}

// LanguageForPath returns the language of a source file by extension.
func LanguageForPath(p string) (Language, bool) {
	l, ok := extLanguages[strings.ToLower(path.Ext(p))]
	return l, ok
}

// Built-in environment names produced by inference.
const (
	EnvDefault     = "default"
	EnvDevelopment = "development"
	EnvTest        = "test"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// NormalizeEnvironment maps common aliases to canonical environment names:
// dev/development/local → development, test/testing → test,
// stage/staging → staging, prod/production → production. Other names are
// returned lower-cased.
func NormalizeEnvironment(name string) string {
	switch n := strings.ToLower(name); n {
	case "dev", "development", "local":
		return EnvDevelopment
	case "test", "testing":
		return EnvTest
	case "stage", "staging":
		return EnvStaging
	case "prod", "production":
		return EnvProduction
	default:
		return n
	}
}

// File is one discovered file.
type File struct {
	// Path is relative to the root, slash-separated.
	Path string
	Kind Kind
	// AppRoot is the nearest ancestor directory (relative, slash-separated,
	// "." for the root) containing a project marker such as package.json or
	// go.mod. It groups env files, templates and code in monorepos.
	AppRoot string
	// Environment is set for env files and templates ("default" for .env and
	// .env.example).
	Environment string
	// Local is true for .env.local and .env.<name>.local overrides.
	Local bool
	// GitIgnored is true when a .gitignore pattern matches the file. Ignored
	// env files are still discovered: rules need them.
	GitIgnored bool
	Language   Language
	Size       int64
}

// SkipReason explains why a path was not included.
type SkipReason string

// Skip reasons.
const (
	SkipBuiltinExclude SkipReason = "builtin-exclude"
	SkipUserExclude    SkipReason = "user-exclude"
	SkipGitIgnored     SkipReason = "gitignored"
	SkipSymlinkOutside SkipReason = "symlink-outside-root"
	SkipSymlinkLoop    SkipReason = "symlink-loop"
	SkipSymlinkSeen    SkipReason = "symlink-duplicate"
	SkipSymlinkBroken  SkipReason = "symlink-broken"
	SkipNotRegular     SkipReason = "not-regular-file"
	SkipMaxDepth       SkipReason = "max-depth"
	SkipMaxFiles       SkipReason = "max-files"
	SkipTooLarge       SkipReason = "too-large"
	SkipBinary         SkipReason = "binary"
	SkipUnreadable     SkipReason = "unreadable"
)

// Skipped records a file or directory that was deliberately not included.
// Only directories and files that would otherwise have been classified are
// recorded, so the list stays small.
type Skipped struct {
	Path   string
	Reason SkipReason
}

// Inventory is the result of a walk. Files and Skipped are sorted by path.
type Inventory struct {
	// Root is the absolute, symlink-resolved root directory.
	Root    string
	Files   []File
	Skipped []Skipped
	// AppRoots lists app root directories that contain at least one file.
	AppRoots []string
	// Truncated is true when the walk stopped at MaxFiles.
	Truncated bool
}

// ByKind returns the files of the given kind, in path order.
func (inv *Inventory) ByKind(k Kind) []File {
	var out []File
	for i := range inv.Files {
		if inv.Files[i].Kind == k {
			out = append(out, inv.Files[i])
		}
	}
	return out
}
