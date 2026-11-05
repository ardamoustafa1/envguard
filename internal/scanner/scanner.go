package scanner

import (
	"bytes"
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// Usage is one read of an environment variable in source code.
type Usage struct {
	// Variable is empty for Dynamic usages.
	Variable string
	// File is relative to the root, slash-separated.
	File string
	// Line and Column (1-based, columns in characters) point at the variable
	// name, or at the start of the expression for Dynamic usages.
	Line, Column int
	Language     discovery.Language
	// HasDefault is true when the read supplies a fallback (`|| "x"`,
	// os.getenv("X", "d"), ${X:-d}, os.LookupEnv, ...).
	HasDefault bool
	// Dynamic is true for computed keys (process.env[name]); such usages
	// never produce findings.
	Dynamic bool
	// Pattern names the detector that matched, e.g. "os.Getenv".
	Pattern string
	// AppRoot is the nearest app root (as in discovery).
	AppRoot string
	// Note is an informational remark, e.g. a client-side read of a variable
	// the framework does not expose to the browser.
	Note string
	// Bulk is set on Dynamic usages that read the whole environment (it is
	// passed to a validator or parser): any declared variable may be read.
	Bulk bool
	// Inferred marks a name guessed from code structure (Go struct tags
	// below an envPrefix). It counts as a read when looking for unused
	// variables, but never as evidence that a variable is undeclared.
	Inferred bool
}

// Certain reports whether the usage names one variable for sure: not a
// computed key and not a guess.
func (u Usage) Certain() bool { return !u.Dynamic && !u.Inferred && u.Variable != "" }

// Options configures Scan.
type Options struct {
	// Root is the absolute repository root.
	Root string
	// Files are discovered files; only KindSource files are scanned.
	Files []discovery.File
	// Workers defaults to GOMAXPROCS.
	Workers int
}

// Result is the outcome of Scan.
type Result struct {
	// Usages are sorted by file, line, column, variable.
	Usages []Usage
	// Scanned is the number of source files read.
	Scanned int
	// Unreadable lists source files that could not be read.
	Unreadable []string
}

// Scan reads every source file once and returns all env var usages. It
// never fails as a whole; unreadable files are listed in Result.Unreadable.
func Scan(opts Options) *Result {
	var files []discovery.File
	for _, f := range opts.Files {
		if f.Kind == discovery.KindSource && languages[f.Language] != nil {
			files = append(files, f)
		}
	}
	frameworks := detectFrameworks(opts.Root, files)
	libraries := detectLibraries(opts.Root, files)

	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	results := make([][]Usage, len(files))
	failed := make([]bool, len(files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(workers, max(len(files), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i], failed[i] = scanOne(opts.Root, files[i], frameworks[files[i].AppRoot], libraries[files[i].AppRoot])
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	res := &Result{Scanned: len(files)}
	for i := range files {
		if failed[i] {
			res.Unreadable = append(res.Unreadable, files[i].Path)
			res.Scanned--
			continue
		}
		res.Usages = append(res.Usages, results[i]...)
	}
	SortUsages(res.Usages)
	return res
}

// scanOne reads and scans one file. A file that cannot be read, or on which
// a detector panics (a bug, but one that must not take the whole run down
// with a goroutine crash dump), is reported as failed and contributes no
// usages: a missed usage is acceptable, a crash is not.
func scanOne(root string, f discovery.File, fw framework, libs library) (us []Usage, failed bool) {
	defer func() {
		if recover() != nil {
			us, failed = nil, true
		}
	}()
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path))) // #nosec G304 -- discovery only returns files inside the root
	if err != nil {
		return nil, true
	}
	us = scanFileWith(f.Language, f.Path, src, libs)
	for k := range us {
		us[k].AppRoot = f.AppRoot
		us[k].Note = fw.note(f, src, &us[k])
	}
	return us, false
}

// SortUsages sorts usages by file, line, column, variable and pattern.
func SortUsages(us []Usage) {
	slices.SortFunc(us, func(a, b Usage) int {
		return cmp.Or(
			strings.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			strings.Compare(a.Variable, b.Variable),
			strings.Compare(a.Pattern, b.Pattern),
		)
	})
}

// fileCtx is the per-file state shared by detectors.
type fileCtx struct {
	lang   discovery.Language
	path   string
	src    []byte
	kinds  []kind
	spans  []span
	starts []int   // line start offsets
	libs   library // libraries the file's app depends on
}

// scanFile finds the usages in one file (without AppRoot or Note).
func scanFile(lang discovery.Language, path string, src []byte) []Usage {
	return scanFileWith(lang, path, src, 0)
}

// scanFileWith is scanFile for a file of an app that depends on libs.
func scanFileWith(lang discovery.Language, path string, src []byte, libs library) []Usage {
	l := languages[lang]
	if l == nil || len(src) == 0 {
		return nil
	}
	kinds, spans := lex(src, l.syntax)
	fc := &fileCtx{lang: lang, path: path, src: src, kinds: kinds, spans: spans, starts: lineStarts(src), libs: libs}
	var out []Usage
	for _, d := range l.detectors {
		out = append(out, fc.apply(d)...)
	}
	for _, x := range l.extra {
		out = append(out, x(fc)...)
	}
	// Different detectors never match the same position, but stay safe.
	SortUsages(out)
	return slices.CompactFunc(out, func(a, b Usage) bool {
		return a.Line == b.Line && a.Column == b.Column && a.Variable == b.Variable
	})
}

const afterWindow = 200

func (fc *fileCtx) apply(d *detector) []Usage {
	src := fc.src
	if d.hint != "" && !bytes.Contains(src, []byte(d.hint)) {
		return nil
	}
	if d.requires != nil && !d.requires.Match(src) && (d.appLib == 0 || fc.libs&d.appLib == 0) {
		return nil
	}
	var out []Usage
	for _, m := range d.re.FindAllSubmatchIndex(src, -1) {
		start, end := m[0], m[1]
		if fc.kinds[start] != kCode || !prevOK(src, start) {
			continue
		}
		if d.before != nil && !d.before.Match(src[max(0, start-40):start]) {
			continue
		}
		tail := src[end:min(end+afterWindow, len(src))]
		if d.skipAfter != nil && d.skipAfter.Match(tail) {
			continue
		}
		u := Usage{Pattern: d.pattern, Language: fc.lang, File: fc.path}
		if d.dynIdx >= 0 && m[2*d.dynIdx] >= 0 {
			u.Dynamic, u.Bulk = true, d.bulk
			u.Line, u.Column = fc.pos(start)
			out = append(out, u)
			continue
		}
		ns, ne := m[2*d.nameIdx], m[2*d.nameIdx+1]
		n := string(src[ns:ne])
		if d.upperOnly && !isUpperName(n) || d.skipNames[n] || hasAnyPrefix(n, d.skipPrefixes) {
			continue
		}
		u.Variable = n
		u.Line, u.Column = fc.pos(ns)
		u.HasDefault = d.alwaysDefault ||
			(d.defIdx >= 0 && m[2*d.defIdx] >= 0 && m[2*d.defIdx+1] > m[2*d.defIdx]) ||
			(d.after != nil && d.after.Match(tail))
		out = append(out, u)
	}
	return out
}

// prevOK rejects matches that are really a member or part of a longer name:
// foo.process.env, myos.Getenv, $this->env(...).
func prevOK(src []byte, start int) bool {
	if start == 0 {
		return true
	}
	p := src[start-1]
	switch {
	case isIdentByte(p), p == '.':
		return false
	case p == '$':
		return src[start] == '$' // only patterns that themselves start with $
	case p == '>' && start >= 2 && src[start-2] == '-':
		return false
	}
	return true
}

func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// pos converts a byte offset to a 1-based line and character column.
func (fc *fileCtx) pos(off int) (line, col int) {
	idx, found := slices.BinarySearch(fc.starts, off)
	if !found {
		idx--
	}
	ls := fc.starts[idx]
	return idx + 1, 1 + utf8.RuneCount(fc.src[ls:off])
}

// --- custom extractors ---

var jsDestructRE = regexp.MustCompile(`\b(?:const|let|var)\s*\{(?P<list>[^{}]*)\}\s*=\s*(?P<src>process\.env|import\.meta\.env|Bun\.env)\b`)

var identRE = regexp.MustCompile(`^[A-Za-z_$][\w$]*`)

// jsDestructuring handles `const { A, B = "d", C: alias, ...rest } = process.env`.
func jsDestructuring(fc *fileCtx) []Usage {
	src := fc.src
	if !bytes.Contains(src, []byte(".env")) {
		return nil
	}
	// Keep offsets, but remove comments and string contents before splitting
	// properties. Their commas and equals signs are not JS syntax here.
	code := bytes.Clone(src)
	for i, k := range fc.kinds {
		if k != kCode && code[i] != '\n' {
			code[i] = ' '
		}
	}
	var out []Usage
	listIdx := jsDestructRE.SubexpIndex("list")
	srcIdx := jsDestructRE.SubexpIndex("src")
	for _, m := range jsDestructRE.FindAllSubmatchIndex(code, -1) {
		if fc.kinds[m[0]] != kCode || !prevOK(src, m[0]) {
			continue
		}
		if m[1] < len(src) && (src[m[1]] == '.' || src[m[1]] == '[') {
			continue // destructuring a property of env, not env itself
		}
		from := string(src[m[2*srcIdx]:m[2*srcIdx+1]])
		pattern := "destructuring " + from
		ls, le := m[2*listIdx], m[2*listIdx+1]
		off := ls
		for _, item := range strings.Split(string(code[ls:le]), ",") {
			itemStart := off
			off += len(item) + 1
			trimmed := strings.TrimLeft(item, " \t\r\n")
			lead := len(item) - len(trimmed)
			trimmed = strings.TrimSpace(trimmed)
			if trimmed == "" {
				continue
			}
			u := Usage{Pattern: pattern, Language: fc.lang, File: fc.path}
			u.Line, u.Column = fc.pos(itemStart + lead)
			if strings.HasPrefix(trimmed, "...") {
				u.Dynamic = true
				out = append(out, u)
				continue
			}
			key := identRE.FindString(trimmed)
			if key == "" || !isUpperName(key) || (from == "import.meta.env" && viteBuiltins[key]) {
				continue
			}
			u.Variable = key
			u.HasDefault = strings.Contains(trimmed[len(key):], "=")
			out = append(out, u)
		}
	}
	return out
}

var (
	goTagRE        = regexp.MustCompile(`(?:^|\s)env:"(?P<name>` + name + `)(?:,[^"]*)?"`)
	goTagDefaultRE = regexp.MustCompile(`(?:^|\s)envDefault:"`)
	goPrefixRE     = regexp.MustCompile(`(?:^|\s)envPrefix:"(` + name + `)"`)
)

// goStructTags handles `env:"X"` struct tags (caarlos0/env and compatible),
// with `envDefault:"..."` as a default. In a file that uses envPrefix the
// real variable names depend on how the structs are nested, which is not
// resolved: every tag name is then reported as Inferred, bare and with every
// prefix of the file, so declared variables are not called unused. Such
// guesses never make a variable "undeclared" (see Usage.Inferred).
func goStructTags(fc *fileCtx) []Usage {
	src := fc.src
	if !bytes.Contains(src, []byte(`env:"`)) {
		return nil
	}
	tags := goFieldTags(src)
	var prefixes []string
	prefixed := false
	if bytes.Contains(src, []byte(`envPrefix:"`)) {
		for _, sp := range tags {
			if sp.open != "`" {
				continue
			}
			body := src[sp.start+1 : max(sp.start+1, sp.end-1)]
			if m := goPrefixRE.FindSubmatch(body); m != nil {
				prefixed = true
				if p := string(m[1]); !slices.Contains(prefixes, p) {
					prefixes = append(prefixes, p)
				}
			}
		}
		slices.Sort(prefixes)
	}
	var out []Usage
	nameIdx := goTagRE.SubexpIndex("name")
	for _, sp := range tags {
		if sp.open != "`" {
			continue
		}
		body := src[sp.start+1 : max(sp.start+1, sp.end-1)]
		m := goTagRE.FindSubmatchIndex(body)
		if m == nil {
			continue
		}
		ns := sp.start + 1 + m[2*nameIdx]
		u := Usage{
			Variable:   string(src[ns : sp.start+1+m[2*nameIdx+1]]),
			Pattern:    "struct tag env",
			Language:   fc.lang,
			File:       fc.path,
			HasDefault: goTagDefaultRE.Match(body),
			Inferred:   prefixed,
		}
		u.Line, u.Column = fc.pos(ns)
		out = append(out, u)
		for _, p := range prefixes {
			pu := u
			pu.Variable = p + u.Variable
			pu.Pattern = "struct tag env (with envPrefix)"
			out = append(out, pu)
		}
	}
	return out
}

// goFieldTags uses Go's syntax tree to distinguish actual struct tags from
// ordinary raw strings, including examples containing env/envPrefix text.
func goFieldTags(src []byte) []span {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	offset := 0
	if f == nil || f.Name == nil || f.Name.Name == "" || f.Name.Name == "_" {
		// Detector tables and editor fragments may omit the package clause.
		const header = "package envguard\n"
		offset = len(header)
		fset = token.NewFileSet()
		f, _ = parser.ParseFile(fset, "", append([]byte(header), src...), parser.SkipObjectResolution)
	}
	if f == nil {
		return nil
	}
	var tags []span
	ast.Inspect(f, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || field.Tag == nil || !strings.HasPrefix(field.Tag.Value, "`") {
			return true
		}
		start := fset.Position(field.Tag.Pos()).Offset - offset
		end := fset.Position(field.Tag.End()).Offset - offset
		if start >= 0 && end <= len(src) && end > start {
			tags = append(tags, span{start: start, end: end, open: "`"})
		}
		return true
	})
	return tags
}

var (
	shellShebangRE = regexp.MustCompile(`^#!.*\b(?:ba|z|da|k)?sh\b`)
	shellAssignRE  = regexp.MustCompile(`(?m)(?:^|[;&|({]|\bthen|\bdo|\belse)[ \t]*(?:(?:export|local|declare|readonly|typeset)[ \t]+(?:-[a-zA-Z]+[ \t]+)*)?(?P<name>[A-Za-z_][A-Za-z0-9_]*)=(?P<rhs>"[^"\n]*"|'[^'\n]*'|[^\s;&|]*)`)
	shellReadRE    = regexp.MustCompile(`\b(?:read(?:[ \t]+-[a-zA-Z]+(?:[ \t]+(?:"[^"\n]*"|'[^'\n]*'))?)*|for|select)[ \t]+(?P<names>[A-Za-z_][A-Za-z0-9_]*(?:[ \t]+[A-Za-z_][A-Za-z0-9_]*)*)`)
	shellUseRE     = regexp.MustCompile(`\$(?:\{(?P<bname>[A-Za-z_][A-Za-z0-9_]*)(?P<op>:?[-=?+])?|(?P<name>[A-Za-z_][A-Za-z0-9_]*)|\{(?P<dyn>!))`)
)

// shellBuiltins are variables the shell itself sets.
var shellBuiltins = set("BASH", "BASHPID", "BASHOPTS", "IFS", "RANDOM", "LINENO", "SECONDS", "PPID", "UID",
	"EUID", "GROUPS", "OPTARG", "OPTIND", "OPTERR", "OLDPWD", "PS1", "PS2", "PS3", "PS4", "SHLVL", "HOSTTYPE",
	"OSTYPE", "MACHTYPE", "PIPESTATUS", "FUNCNAME", "REPLY", "COLUMNS", "LINES", "HISTFILE", "HISTSIZE",
	"HISTFILESIZE", "DIRSTACK", "EPOCHSECONDS", "EPOCHREALTIME", "SRANDOM", "COMP_WORDS", "COMP_CWORD", "ZSH_VERSION")

// shellUsages handles ${X}, ${X:-default} and $X in shell scripts. Only
// UPPER_CASE names count, variables the script assigns itself (X=..., read X,
// for X in) are not env usages, and shell built-ins are ignored. `X=${X:-d}`
// is an env read with a default, not an assignment that hides X.
func shellUsages(fc *fileCtx) []Usage {
	src := fc.src
	if bytes.HasPrefix(src, []byte("#!")) && !shellShebangRE.Match(src[:lineEnd(src, len(src), 0)]) {
		return nil // e.g. a .sh file with a python shebang
	}
	assigned := map[string]bool{}
	ni, ri := shellAssignRE.SubexpIndex("name"), shellAssignRE.SubexpIndex("rhs")
	for _, m := range shellAssignRE.FindAllSubmatchIndex(src, -1) {
		if fc.kinds[m[2*ni]] != kCode {
			continue
		}
		n := string(src[m[2*ni]:m[2*ni+1]])
		rhs := string(src[m[2*ri]:m[2*ri+1]])
		if strings.Contains(rhs, "${"+n) || strings.Contains(rhs, "$"+n) {
			continue // X=${X:-default}: reads the environment
		}
		assigned[n] = true
	}
	nsIdx := shellReadRE.SubexpIndex("names")
	for _, m := range shellReadRE.FindAllSubmatchIndex(src, -1) {
		if fc.kinds[m[0]] != kCode {
			continue
		}
		for _, n := range strings.Fields(string(src[m[2*nsIdx]:m[2*nsIdx+1]])) {
			assigned[n] = true
		}
	}

	var out []Usage
	bi, oi := shellUseRE.SubexpIndex("bname"), shellUseRE.SubexpIndex("op")
	pi, di := shellUseRE.SubexpIndex("name"), shellUseRE.SubexpIndex("dyn")
	for _, m := range shellUseRE.FindAllSubmatchIndex(src, -1) {
		start := m[0]
		if fc.kinds[start] != kCode || (start > 0 && (src[start-1] == '\\' || src[start-1] == '$')) {
			continue
		}
		u := Usage{Language: fc.lang, File: fc.path}
		if m[2*di] >= 0 {
			u.Dynamic, u.Pattern = true, "shell ${!X}"
			u.Line, u.Column = fc.pos(start)
			out = append(out, u)
			continue
		}
		ns, ne := m[2*bi], m[2*bi+1]
		u.Pattern = "shell ${X}"
		if ns < 0 {
			ns, ne = m[2*pi], m[2*pi+1]
			u.Pattern = "shell $X"
		}
		n := string(src[ns:ne])
		if !isUpperName(n) || assigned[n] || shellBuiltins[n] || strings.HasPrefix(n, "BASH_") || n == "_" {
			continue
		}
		if m[2*oi] >= 0 {
			op := string(src[m[2*oi]:m[2*oi+1]])
			u.HasDefault = strings.TrimPrefix(op, ":") != "?"
		}
		u.Variable = n
		u.Line, u.Column = fc.pos(ns)
		out = append(out, u)
	}
	return out
}

// --- well-known variables ---

var wellKnownExact = set("PATH", "HOME", "USER", "SHELL", "PWD", "TERM", "LANG", "TZ", "TMPDIR", "CI",
	"NODE_ENV", "HOSTNAME", "AWS_REGION", "AWS_DEFAULT_REGION",
	// Cross-tool terminal conventions set by the user, not app config.
	"NO_COLOR", "FORCE_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "COLORTERM", "TERM_PROGRAM", "COLUMNS", "LINES",
	// Standard ambient variables of Unix and Windows sessions.
	"LOGNAME", "LANGUAGE", "EDITOR", "VISUAL", "PAGER", "DISPLAY", "SSH_AUTH_SOCK", "TMP", "TEMP", "OLDPWD",
	"APPDATA", "LOCALAPPDATA", "USERPROFILE", "USERNAME", "HOMEDRIVE", "HOMEPATH", "SYSTEMROOT", "WINDIR", "COMSPEC",
	"PROGRAMFILES", "PROGRAMDATA", "OS", "PROCESSOR_ARCHITECTURE", "WT_SESSION", "ConEmuANSI",
	// Proxy conventions honored by most HTTP clients.
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy",
	// Set by the hosting platform or the test runner, never by the app's config.
	"VERCEL", "VITEST", "JEST_WORKER_ID", "NETLIFY", "RENDER", "DYNO", "CODESPACES", "GITLAB_CI", "TRAVIS",
	"CIRCLECI", "BUILDKITE", "JENKINS_URL", "TF_BUILD", "TEAMCITY_VERSION")

var wellKnownPrefixes = []string{"GITHUB_", "RUNNER_", "VERCEL_", "NETLIFY_", "RENDER_", "FLY_", "HEROKU_", "KUBERNETES_SERVICE_",
	"XDG_", "LC_", "CI_", "VITEST_", "npm_config_", "npm_package_", "npm_lifecycle_"}

// IsWellKnown reports whether a variable is provided by the OS, the shell or
// a CI/hosting platform, so EG003 must not ask for it to be declared. PORT
// counts only when the code supplies a default. AWS credentials are
// deliberately not on the list.
func IsWellKnown(variable string, hasDefault bool) bool {
	if wellKnownExact[variable] || hasAnyPrefix(variable, wellKnownPrefixes) {
		return true
	}
	return variable == "PORT" && hasDefault
}
