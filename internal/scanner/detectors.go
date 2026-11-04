package scanner

import (
	"regexp"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// name is the regex for a variable name inside a quoted key.
const name = `[A-Za-z_][A-Za-z0-9_]*`

// Detector regexes deliberately do not start with \b: Go's regexp can only use
// its fast literal-prefix search when the pattern starts with a literal, and
// \b would force a full match attempt at every byte of every file (about 3x
// slower on a large repository). prevOK, applied to every match, already
// rejects a match glued to a longer identifier (xprocess.env, foo.process.env).
//
// detector is one usage pattern. re must contain one of the named groups
// "name" (the variable) or "dyn" (a computed key: Dynamic usage), and may
// contain "def" (non-empty match means the read supplies a default).
type detector struct {
	pattern string         // reported in Usage.Pattern
	hint    string         // literal that must occur in the file (fast pre-filter)
	re      *regexp.Regexp // the usage
	// requires gates the detector on a file-level match (e.g. an import).
	requires *regexp.Regexp
	// appLib additionally lets the detector run in a file that does not match
	// requires when the app's dependency files name the library (e.g. Django
	// settings split over several modules, of which only one imports environ).
	appLib library
	// before, if set, must match the text right before the usage (the
	// regexp is anchored at its end): a stand-in for lookbehind.
	before *regexp.Regexp
	// bulk marks a dyn usage that reads the whole environment (the object is
	// passed to a function), not one computed key.
	bulk bool
	// after is matched at the end of the usage; a match means a default
	// follows (`|| "x"`, `?? "x"`, `.unwrap_or(...)`).
	after *regexp.Regexp
	// skipAfter is matched at the end of the usage; a match means the
	// expression is not a read (an assignment or a method call).
	skipAfter     *regexp.Regexp
	alwaysDefault bool // the API itself tolerates absence (LookupEnv, option_env!)
	upperOnly     bool // property-style access: only UPPER_CASE names count
	skipNames     map[string]bool
	skipPrefixes  []string

	nameIdx, defIdx, dynIdx int
}

func newDetector(d detector) *detector {
	d.nameIdx = d.re.SubexpIndex("name")
	d.defIdx = d.re.SubexpIndex("def")
	d.dynIdx = d.re.SubexpIndex("dyn")
	if d.nameIdx < 0 && d.dynIdx < 0 {
		panic("scanner: detector " + d.pattern + " has neither name nor dyn group")
	}
	return &d
}

func set(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

var (
	// A default follows (|| ??), or the value is compared with ==, ===, != or
	// !==: the code handles absence by branching, so the read is optional.
	jsDefault = regexp.MustCompile(`^(?:!?\s*(?:\|\||\?\?)|\s*[!=]==?)`)
	// Python/Go/Ruby/Java style comparisons after the read: `== "1"`, `!= ""`.
	cmpAfter      = regexp.MustCompile(`^\s*[!=]=`)
	pyCallCmp     = regexp.MustCompile(`^\s*\)\s*(?:[!=]=|\bin\b|\bnot\s+in\b)`)
	jsSkip        = regexp.MustCompile(`^\s*(?:\(|=[^=>])`)
	assignSkip    = regexp.MustCompile(`^\s*=[^=~>]`)
	viteBuiltins  = set("MODE", "BASE_URL", "PROD", "DEV", "SSR")
	quotedName    = `["'](?P<name>` + name + `)["']`
	notQuoteOrEnd = `(?P<dyn>[^"'\x60\s)\]]|\x60[^\x60]*\$\{)`
)

// language families.
type language struct {
	syntax    *syntax
	detectors []*detector
	// extra extracts usages that need custom parsing.
	extra []func(fc *fileCtx) []Usage
}

var cSlash = []blockComment{{open: "/*", close: "*/"}}

var jsSyntax = &syntax{
	line:       []string{"//"},
	block:      cSlash,
	quotes:     []quote{{open: `"`, close: `"`, escape: true}, {open: `'`, close: `'`, escape: true}},
	jsTemplate: true,
	jsRegex:    true,
}

var jsDetectors = []*detector{
	newDetector(detector{pattern: "process.env.X", hint: "process.env",
		re:    regexp.MustCompile(`process\.env\.(?P<name>[A-Za-z_$][\w$]*)`),
		after: jsDefault, skipAfter: jsSkip, upperOnly: true}),
	newDetector(detector{pattern: `process.env["X"]`, hint: "process.env",
		re:    regexp.MustCompile(`process\.env\[\s*["'\x60](?P<name>` + name + `)["'\x60]\s*\]`),
		after: jsDefault, skipAfter: assignSkip}),
	newDetector(detector{pattern: "process.env[expr]", hint: "process.env",
		re: regexp.MustCompile(`process\.env\[\s*` + notQuoteOrEnd)}),
	// The whole environment handed to a validator (schema.parse(process.env),
	// createEnv({ runtimeEnv: process.env }), function load(source = process.env)):
	// the variable names live in the schema, so which ones are read cannot be known.
	newDetector(detector{pattern: "process.env (passed whole)", hint: "process.env", bulk: true,
		re:     regexp.MustCompile(`process\.env(?P<dyn>\s*[,)])`),
		before: regexp.MustCompile(`(?:[(,=]|\bruntimeEnv(?:Strict)?\s*:)\s*$`)}),
	newDetector(detector{pattern: "import.meta.env.X", hint: "import.meta.env",
		re:    regexp.MustCompile(`import\.meta\.env\.(?P<name>[A-Za-z_$][\w$]*)`),
		after: jsDefault, skipAfter: jsSkip, upperOnly: true, skipNames: viteBuiltins}),
	newDetector(detector{pattern: `import.meta.env["X"]`, hint: "import.meta.env",
		re:    regexp.MustCompile(`import\.meta\.env\[\s*["'\x60](?P<name>` + name + `)["'\x60]\s*\]`),
		after: jsDefault, skipAfter: assignSkip, skipNames: viteBuiltins}),
	newDetector(detector{pattern: "import.meta.env[expr]", hint: "import.meta.env",
		re: regexp.MustCompile(`import\.meta\.env\[\s*` + notQuoteOrEnd)}),
	newDetector(detector{pattern: "Deno.env.get", hint: "Deno.env",
		re:    regexp.MustCompile(`Deno\.env\.get\(\s*["'\x60](?P<name>` + name + `)["'\x60]\s*\)`),
		after: jsDefault}),
	newDetector(detector{pattern: "Deno.env.get(expr)", hint: "Deno.env",
		re: regexp.MustCompile(`Deno\.env\.get\(\s*` + notQuoteOrEnd)}),
	newDetector(detector{pattern: "Bun.env.X", hint: "Bun.env",
		re:    regexp.MustCompile(`Bun\.env\.(?P<name>[A-Za-z_$][\w$]*)`),
		after: jsDefault, skipAfter: jsSkip, upperOnly: true}),
	newDetector(detector{pattern: `Bun.env["X"]`, hint: "Bun.env",
		re:    regexp.MustCompile(`Bun\.env\[\s*["'\x60](?P<name>` + name + `)["'\x60]\s*\]`),
		after: jsDefault, skipAfter: assignSkip}),
}

var pySyntax = &syntax{
	line: []string{"#"},
	quotes: []quote{
		{open: `"""`, close: `"""`, escape: true, multiline: true},
		{open: `'''`, close: `'''`, escape: true, multiline: true},
		{open: `"`, close: `"`, escape: true},
		{open: `'`, close: `'`, escape: true},
	},
}

var (
	fromOSImportEnviron = regexp.MustCompile(`(?m)^\s*from\s+os\s+import\s+[^\n]*\benviron\b`)
	djangoEnviron       = regexp.MustCompile(`import\s+environ\b|\bfrom\s+environ\s+import\b`)
)

var pyDetectors = []*detector{
	newDetector(detector{pattern: `os.environ["X"]`, hint: "environ",
		re: regexp.MustCompile(`os\.environ\[\s*` + quotedName + `\s*\]`), skipAfter: assignSkip}),
	newDetector(detector{pattern: "os.environ.get", hint: "environ",
		re: regexp.MustCompile(`os\.environ\.get\(\s*` + quotedName + `\s*(?P<def>,)?`), after: pyCallCmp}),
	newDetector(detector{pattern: "os.getenv", hint: "getenv",
		re: regexp.MustCompile(`os\.getenv\(\s*` + quotedName + `\s*(?P<def>,)?`), after: pyCallCmp}),
	newDetector(detector{pattern: "os.environ[expr]", hint: "environ",
		re: regexp.MustCompile(`os\.environ\[\s*` + notQuoteOrEnd)}),
	newDetector(detector{pattern: "os.getenv(expr)", hint: "os.",
		re: regexp.MustCompile(`os\.(?:getenv|environ\.get)\(\s*` + notQuoteOrEnd)}),
	newDetector(detector{pattern: `environ["X"]`, hint: "environ", requires: fromOSImportEnviron,
		re: regexp.MustCompile(`environ\[\s*` + quotedName + `\s*\]`), skipAfter: assignSkip}),
	newDetector(detector{pattern: "environ.get", hint: "environ", requires: fromOSImportEnviron,
		re: regexp.MustCompile(`environ\.get\(\s*` + quotedName + `\s*(?P<def>,)?`)}),
	// Lower confidence: django-environ's env("X", default=...). Gated on the
	// file importing environ; only an explicit default= counts as a default
	// (its second positional argument is a cast, not a default).
	newDetector(detector{pattern: "django-environ env()", hint: "env", requires: djangoEnviron, appLib: libDjangoEnviron,
		re: regexp.MustCompile(`env(?:\.(?:str|bool|int|float|list|tuple|dict|json|url|db_url|db|cache_url|cache|email_url|email|search_url|path))?\(\s*` +
			quotedName),
		after: regexp.MustCompile(`^\s*,[^)]*\bdefault\s*=`)}),
}

var goSyntax = &syntax{
	line:  []string{"//"},
	block: cSlash,
	quotes: []quote{
		{open: "`", close: "`", multiline: true},
		{open: `"`, close: `"`, escape: true},
		{open: `'`, close: `'`, escape: true},
	},
}

var goDetectors = []*detector{
	newDetector(detector{pattern: "os.Getenv", hint: "Getenv",
		re: regexp.MustCompile(`os\.Getenv\(\s*"(?P<name>` + name + `)"\s*\)`), after: cmpAfter}),
	newDetector(detector{pattern: "os.LookupEnv", hint: "LookupEnv", alwaysDefault: true,
		re: regexp.MustCompile(`os\.LookupEnv\(\s*"(?P<name>` + name + `)"\s*\)`)}),
	newDetector(detector{pattern: "os.Getenv(expr)", hint: "os.",
		re: regexp.MustCompile(`os\.(?:Getenv|LookupEnv)\(\s*(?P<dyn>[^"\s)])`)}),
}

var rubySyntax = &syntax{
	line:  []string{"#"},
	block: []blockComment{{open: "=begin", close: "=end", lineStart: true}},
	quotes: []quote{
		{open: `"`, close: `"`, escape: true, multiline: true},
		{open: `'`, close: `'`, escape: true, multiline: true},
	},
}

var rubyDetectors = []*detector{
	newDetector(detector{pattern: `ENV["X"]`, hint: "ENV",
		re:        regexp.MustCompile(`ENV\[\s*` + quotedName + `\s*\]`),
		after:     regexp.MustCompile(`^\s*(?:\|\||\.presence\s*\|\||[!=]=)`),
		skipAfter: regexp.MustCompile(`^\s*=[^=~>]`)}),
	newDetector(detector{pattern: "ENV.fetch", hint: "ENV",
		re:    regexp.MustCompile(`ENV\.fetch(?:\(\s*|\s+)` + quotedName + `\s*(?P<def>,)?`),
		after: regexp.MustCompile(`^\)\s*(?:\{|do\b)`)}),
	newDetector(detector{pattern: "ENV[expr]", hint: "ENV",
		re: regexp.MustCompile(`ENV(?:\[\s*|\.fetch\(\s*)` + notQuoteOrEnd)}),
}

var phpSyntax = &syntax{
	line:    []string{"//", "#"},
	block:   cSlash,
	quotes:  []quote{{open: `"`, close: `"`, escape: true, multiline: true}, {open: `'`, close: `'`, escape: true, multiline: true}},
	regions: phpRegions,
}

var phpDetectors = []*detector{
	newDetector(detector{pattern: "getenv", hint: "getenv",
		re:    regexp.MustCompile(`getenv\(\s*` + quotedName),
		after: regexp.MustCompile(`^\s*(?:,[^)]*)?\)\s*(?:\?:|\?\?|\|\||[!=]==?)`)}),
	newDetector(detector{pattern: `$_ENV["X"]`, hint: "$_ENV",
		re:    regexp.MustCompile(`\$_ENV\[\s*` + quotedName + `\s*\]`),
		after: regexp.MustCompile(`^\s*\?\?`), skipAfter: assignSkip}),
	// $_SERVER also holds request data; only names that are not
	// request/server metadata count.
	newDetector(detector{pattern: `$_SERVER["X"]`, hint: "$_SERVER",
		re:    regexp.MustCompile(`\$_SERVER\[\s*` + quotedName + `\s*\]`),
		after: regexp.MustCompile(`^\s*\?\?`), skipAfter: assignSkip,
		skipNames: set("QUERY_STRING", "DOCUMENT_ROOT", "PHP_SELF", "PATH_INFO", "ORIG_PATH_INFO", "HTTPS",
			"CONTENT_TYPE", "CONTENT_LENGTH", "GATEWAY_INTERFACE", "AUTH_TYPE", "argv", "argc", "PATH_TRANSLATED"),
		skipPrefixes: []string{"HTTP_", "REQUEST_", "SERVER_", "SCRIPT_", "REMOTE_", "PHP_AUTH_", "REDIRECT_"}}),
	// Laravel's env() helper returns its default, or null: it never fails, so
	// every read is optional by contract (the stock config/*.php files read
	// dozens of variables without a default).
	newDetector(detector{pattern: "env() (Laravel)", hint: "env(", alwaysDefault: true,
		re: regexp.MustCompile(`env\(\s*` + quotedName + `\s*(?P<def>,)?`)}),
	newDetector(detector{pattern: "getenv(expr)", hint: "$",
		re: regexp.MustCompile(`(?:\bgetenv\(|\$_ENV\[|\benv\()\s*(?P<dyn>\$)`)}),
}

var rustSyntax = &syntax{
	line:     []string{"//"},
	block:    []blockComment{{open: "/*", close: "*/", nested: true}},
	quotes:   []quote{{open: `"`, close: `"`, escape: true, multiline: true}},
	rustRaw:  true,
	rustChar: true,
}

var rustDetectors = []*detector{
	newDetector(detector{pattern: "env::var", hint: "env::var",
		re:    regexp.MustCompile(`(?:std::)?env::var(?:_os)?\(\s*"(?P<name>` + name + `)"\s*\)`),
		after: regexp.MustCompile(`^\s*\.\s*(?:unwrap_or(?:_else|_default)?|ok|map_or(?:_else)?|is_ok|is_err)\b`)}),
	newDetector(detector{pattern: "env!", hint: "env!",
		re: regexp.MustCompile(`env!\(\s*"(?P<name>` + name + `)"`)}),
	newDetector(detector{pattern: "option_env!", hint: "option_env!", alwaysDefault: true,
		re: regexp.MustCompile(`option_env!\(\s*"(?P<name>` + name + `)"`)}),
	newDetector(detector{pattern: "env::var(expr)", hint: "env::var",
		re: regexp.MustCompile(`(?:std::)?env::var(?:_os)?\(\s*(?P<dyn>[^"\s)])`)}),
}

var javaSyntax = &syntax{
	line:  []string{"//"},
	block: cSlash,
	quotes: []quote{
		{open: `"""`, close: `"""`, escape: true, multiline: true},
		{open: `"`, close: `"`, escape: true},
		{open: `'`, close: `'`, escape: true},
	},
}

var javaDetectors = []*detector{
	newDetector(detector{pattern: "System.getenv", hint: "getenv",
		re:    regexp.MustCompile(`System\.getenv\(\s*"(?P<name>` + name + `)"\s*\)`),
		after: regexp.MustCompile(`^\s*(?:\?:|[!=]=)`)}), // Kotlin elvis, comparison
	newDetector(detector{pattern: "System.getenv().getOrDefault", hint: "getOrDefault", alwaysDefault: true,
		re: regexp.MustCompile(`System\.getenv\(\)\s*\.getOrDefault\(\s*"(?P<name>` + name + `)"`)}),
	newDetector(detector{pattern: "System.getenv(expr)", hint: "getenv",
		re: regexp.MustCompile(`System\.getenv\(\s*(?P<dyn>[^"\s)])`)}),
}

var csharpSyntax = &syntax{
	line:  []string{"//"},
	block: cSlash,
	quotes: []quote{
		{open: `$@"`, close: `"`, multiline: true},
		{open: `@$"`, close: `"`, multiline: true},
		{open: `@"`, close: `"`, multiline: true},
		{open: `"""`, close: `"""`, multiline: true},
		{open: `"`, close: `"`, escape: true},
		{open: `'`, close: `'`, escape: true},
	},
}

var csharpDetectors = []*detector{
	newDetector(detector{pattern: "Environment.GetEnvironmentVariable", hint: "GetEnvironmentVariable",
		re:    regexp.MustCompile(`Environment\.GetEnvironmentVariable\(\s*"(?P<name>` + name + `)"\s*(?:,[^)]*)?\)`),
		after: regexp.MustCompile(`^\s*\?\?`)}),
	newDetector(detector{pattern: "Environment.GetEnvironmentVariable(expr)", hint: "GetEnvironmentVariable",
		re: regexp.MustCompile(`Environment\.GetEnvironmentVariable\(\s*(?P<dyn>[^"\s)@$])`)}),
}

var elixirSyntax = &syntax{
	line: []string{"#"},
	quotes: []quote{
		{open: `"""`, close: `"""`, escape: true, multiline: true},
		{open: `'''`, close: `'''`, escape: true, multiline: true},
		{open: `"`, close: `"`, escape: true, multiline: true},
		{open: `'`, close: `'`, escape: true, multiline: true},
	},
}

var elixirDetectors = []*detector{
	newDetector(detector{pattern: "System.get_env", hint: "get_env",
		re:    regexp.MustCompile(`System\.get_env\(\s*"(?P<name>` + name + `)"\s*(?P<def>,)?`),
		after: regexp.MustCompile(`^\s*\)\s*\|\|`)}),
	newDetector(detector{pattern: "System.fetch_env!", hint: "fetch_env!",
		re: regexp.MustCompile(`System\.fetch_env!\(\s*"(?P<name>` + name + `)"\s*\)`)}),
	newDetector(detector{pattern: "System.get_env(expr)", hint: "get_env",
		re: regexp.MustCompile(`System\.(?:get_env|fetch_env!?)\(\s*(?P<dyn>[^"\s)])`)}),
}

var shellSyntax = &syntax{
	line:         []string{"#"},
	hashBoundary: true,
	quotes: []quote{
		{open: `$'`, close: `'`, escape: true, multiline: true},
		{open: `'`, close: `'`, multiline: true},
		{open: `"`, close: `"`, escape: true, multiline: true, keepCode: true},
		{open: "`", close: "`", escape: true, multiline: true, keepCode: true},
	},
}

var languages = map[discovery.Language]*language{
	discovery.LangJavaScript: {syntax: jsSyntax, detectors: jsDetectors, extra: []func(*fileCtx) []Usage{jsDestructuring}},
	discovery.LangTypeScript: {syntax: jsSyntax, detectors: jsDetectors, extra: []func(*fileCtx) []Usage{jsDestructuring}},
	discovery.LangVue:        {syntax: withRegions(jsSyntax, scriptRegions), detectors: jsDetectors, extra: []func(*fileCtx) []Usage{jsDestructuring}},
	discovery.LangSvelte:     {syntax: withRegions(jsSyntax, scriptRegions), detectors: jsDetectors, extra: []func(*fileCtx) []Usage{jsDestructuring}},
	discovery.LangPython:     {syntax: pySyntax, detectors: pyDetectors},
	discovery.LangGo:         {syntax: goSyntax, detectors: goDetectors, extra: []func(*fileCtx) []Usage{goStructTags}},
	discovery.LangRuby:       {syntax: rubySyntax, detectors: rubyDetectors},
	discovery.LangPHP:        {syntax: phpSyntax, detectors: phpDetectors},
	discovery.LangRust:       {syntax: rustSyntax, detectors: rustDetectors},
	discovery.LangJava:       {syntax: javaSyntax, detectors: javaDetectors},
	discovery.LangKotlin:     {syntax: javaSyntax, detectors: javaDetectors},
	discovery.LangCSharp:     {syntax: csharpSyntax, detectors: csharpDetectors},
	discovery.LangElixir:     {syntax: elixirSyntax, detectors: elixirDetectors},
	discovery.LangShell:      {syntax: shellSyntax, extra: []func(*fileCtx) []Usage{shellUsages}},
}

func withRegions(sx *syntax, regions func([]byte) [][2]int) *syntax {
	c := *sx
	c.regions = regions
	return &c
}

func isUpperName(s string) bool {
	hasLetter := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			hasLetter = true
		case c == '_' || c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return hasLetter
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
