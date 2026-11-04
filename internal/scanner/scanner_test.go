package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// render formats usages compactly: NAME@line:col, "+d" for a default,
// "dyn@line:col" for dynamic usages ("bulk@" for whole-environment reads) and
// a "~" after the name for inferred ones.
func render(us []Usage) []string {
	var out []string
	for _, u := range us {
		s := u.Variable
		if u.Inferred {
			s += "~"
		}
		if u.Dynamic {
			s = "dyn"
			if u.Bulk {
				s = "bulk"
			}
		}
		s += fmt.Sprintf("@%d:%d", u.Line, u.Column)
		if u.HasDefault {
			s += "+d"
		}
		out = append(out, s)
	}
	return out
}

type tc struct {
	name string
	src  string
	want []string
}

func runTable(t *testing.T, lang discovery.Language, tests []tc) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := render(scanFile(lang, "f", []byte(tt.src)))
			if !slices.Equal(got, tt.want) {
				t.Errorf("source:\n%s\ngot  %v\nwant %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestJavaScript(t *testing.T) {
	runTable(t, discovery.LangJavaScript, []tc{
		{"dot", `const a = process.env.API_URL`, []string{"API_URL@1:23"}},
		{"bracket double", `x(process.env["DB_URL"])`, []string{"DB_URL@1:16"}},
		{"bracket single", `x(process.env['DB_URL'])`, []string{"DB_URL@1:16"}},
		{"or default", `const p = process.env.PORT || 3000`, []string{"PORT@1:23+d"}},
		{"nullish default", `const p = process.env.PORT ?? "3000"`, []string{"PORT@1:23+d"}},
		{"ts non-null then default", `const p = process.env.PORT! ?? "3000"`, []string{"PORT@1:23+d"}},
		{"no default with &&", `if (process.env.FLAG && x) {}`, []string{"FLAG@1:17"}},
		{"destructuring", "const { A, B = \"d\", C: c, D: d2 = 1 } = process.env", []string{"A@1:9", "B@1:12+d", "C@1:21", "D@1:27+d"}},
		{"destructuring multiline", "const {\n  API_KEY,\n  ...rest\n} = process.env", []string{"API_KEY@2:3", "dyn@3:3"}},
		{"destructuring property is not env", "const { a } = process.env.CONFIG", []string{"CONFIG@1:27"}},
		{"import.meta.env", `const u = import.meta.env.VITE_API_URL`, []string{"VITE_API_URL@1:27"}},
		{"vite builtins ignored", `if (import.meta.env.DEV || import.meta.env.MODE) {}`, nil},
		{"deno", `Deno.env.get("TOKEN") ?? "x"`, []string{"TOKEN@1:15+d"}},
		{"bun", `Bun.env.SECRET_KEY`, []string{"SECRET_KEY@1:9"}},
		{"dynamic bracket", `process.env[name]; process.env[` + "`X_${y}`" + `]`, []string{"dyn@1:1", "dyn@1:20"}},
		{"deno dynamic", `Deno.env.get(key)`, []string{"dyn@1:1"}},
		{"line comment", `// process.env.FAKE_A`, nil},
		{"block comment", "/* process.env.FAKE_A\n process.env.FAKE_B */ process.env.REAL", []string{"REAL@2:36"}},
		{"double string", `console.log("process.env.FAKE_A")`, nil},
		{"single string", `log('use process.env.FAKE_A here')`, nil},
		{"template text", "log(`process.env.FAKE_A`)", nil},
		{"template interpolation is code", "log(`url=${process.env.REAL_URL}/x ${ {a:1}.a } process.env.FAKE`)", []string{"REAL_URL@1:24"}},
		{"nested template", "`a ${`b ${process.env.REAL} process.env.FAKE`} c`", []string{"REAL@1:23"}},
		{"regex literal", `const r = /process.env.FAKE_A/; process.env.REAL`, []string{"REAL@1:45"}},
		{"regex with quote", `s.replace(/"/g, "'"); process.env.REAL`, []string{"REAL@1:35"}},
		{"division not regex", `const x = a / b; const y = c / d; process.env.REAL`, []string{"REAL@1:47"}},
		{"assignment is not a read", `process.env.FOO = "bar"; process.env["BAR"] = 1`, nil},
		{"comparison handles absence, so it is optional", `if (process.env.FOO === "x") {}`, []string{"FOO@1:17+d"}},
		{"method call ignored", `process.env.hasOwnProperty("X")`, nil},
		{"lowercase dot ignored", `process.env.npm_config_x`, nil},
		{"member of other object", `foo.process.env.FAKE; myprocess.env.FAKE2`, nil},
		{"escaped quote in string", `log("a \" process.env.FAKE"); process.env.REAL`, []string{"REAL@1:43"}},
		{"unicode column", `const s = "日本"; const x = process.env.REAL`, []string{"REAL@1:39"}},
	})
}

func TestVueOnlyScript(t *testing.T) {
	src := "<template>\n  <p>Don't use process.env.FAKE here</p>\n</template>\n<script setup lang=\"ts\">\nconst u = import.meta.env.VITE_URL\n</script>\n"
	got := render(scanFile(discovery.LangVue, "c.vue", []byte(src)))
	if !slices.Equal(got, []string{"VITE_URL@5:27"}) {
		t.Errorf("got %v", got)
	}
}

func TestPython(t *testing.T) {
	runTable(t, discovery.LangPython, []tc{
		{"environ index", `x = os.environ["DATABASE_URL"]`, []string{"DATABASE_URL@1:17"}},
		{"environ get", `x = os.environ.get('DEBUG')`, []string{"DEBUG@1:21"}},
		{"environ get default", `x = os.environ.get("DEBUG", "0")`, []string{"DEBUG@1:21+d"}},
		{"getenv default", `x = os.getenv("PORT", 8000)`, []string{"PORT@1:16+d"}},
		{"getenv", `x = os.getenv("SECRET")`, []string{"SECRET@1:16"}},
		{"assignment not read", `os.environ["X"] = "1"`, nil},
		{"comment", `# os.getenv("FAKE")`, nil},
		{"docstring", "def f():\n    \"\"\"Reads os.getenv('FAKE') at startup.\"\"\"\n    return os.getenv('REAL')", []string{"REAL@3:23"}},
		{"single-quoted docstring", "'''\nos.environ['FAKE']\n'''\nos.environ['REAL']", []string{"REAL@4:13"}},
		{"string", `print("os.getenv('FAKE')")`, nil},
		{"f-string is skipped (precision)", `print(f"{os.getenv('MISSED')}")`, nil},
		{"dynamic", `os.getenv(name); os.environ[key]; os.environ.get(k)`, []string{"dyn@1:1", "dyn@1:18", "dyn@1:35"}},
		{"from os import environ", "from os import environ, path\nx = environ['DB']\ny = environ.get('Z', 1)", []string{"DB@2:14", "Z@3:18+d"}},
		{"bare environ without import", `x = environ['FAKE']`, nil},
		{"django-environ", "import environ\nenv = environ.Env()\nA = env('A')\nB = env.bool('B', default=False)\nC = env.str('C', str)", []string{"A@3:10", "B@4:15+d", "C@5:14"}},
		{"env() without environ import", `A = env('FAKE')`, nil},
	})
}

func TestGo(t *testing.T) {
	runTable(t, discovery.LangGo, []tc{
		{"getenv", `dsn := os.Getenv("DATABASE_URL")`, []string{"DATABASE_URL@1:19"}},
		{"lookup env", `v, ok := os.LookupEnv("PORT")`, []string{"PORT@1:24+d"}},
		{"dynamic", `os.Getenv(key)`, []string{"dyn@1:1"}},
		{"comment", "// os.Getenv(\"FAKE\")\n/* os.Getenv(\"FAKE2\") */", nil},
		{"string", "fmt.Println(\"os.Getenv(\\\"FAKE\\\")\")", nil},
		{"raw string", "x := `os.Getenv(\"FAKE\")`", nil},
		{"rune literal quote", `c := '"'; x := os.Getenv("REAL")`, []string{"REAL@1:27"}},
		{"struct tags", "type C struct {\n\tPort int `env:\"PORT\" envDefault:\"8080\"`\n\tDB string `env:\"DATABASE_URL,required\"`\n\tName string `json:\"name\"`\n}",
			[]string{"PORT@2:17+d", "DATABASE_URL@3:18"}},
		// With envPrefix the real names depend on nesting: both candidates are
		// reported as inferred, never as certain.
		{"envPrefix names are inferred", "type C struct {\n\tDB DB `envPrefix:\"DB_\"`\n}\ntype DB struct {\n\tHost string `env:\"HOST\"`\n}",
			[]string{"DB_HOST~@5:20", "HOST~@5:20"}},
		{"several prefixes", "type C struct {\n\tA A `envPrefix:\"A_\"`\n\tB A `envPrefix:\"B_\"`\n\tN string `env:\"N\" envDefault:\"x\"`\n}",
			[]string{"A_N~@4:17+d", "B_N~@4:17+d", "N~@4:17+d"}},
		{"comparison is optional", `if os.Getenv("DEBUG") == "1" || os.Getenv("TRACE") != "" {}`, []string{"DEBUG@1:15+d", "TRACE@1:44+d"}},
		{"other package", `x := myos.Getenv("FAKE")`, nil},
	})
}

func TestRuby(t *testing.T) {
	runTable(t, discovery.LangRuby, []tc{
		{"index", `url = ENV["REDIS_URL"]`, []string{"REDIS_URL@1:12"}},
		{"index default", `url = ENV['REDIS_URL'] || "redis://localhost"`, []string{"REDIS_URL@1:12+d"}},
		{"fetch", `ENV.fetch("SECRET_KEY_BASE")`, []string{"SECRET_KEY_BASE@1:12"}},
		{"fetch default", `ENV.fetch("PORT", 3000)`, []string{"PORT@1:12+d"}},
		{"fetch block", `ENV.fetch("PORT") { 3000 }`, []string{"PORT@1:12+d"}},
		{"fetch do block", "ENV.fetch(\"PORT\") do\n 3000\nend", []string{"PORT@1:12+d"}},
		{"fetch without parens", `ENV.fetch "HOST", "x"`, []string{"HOST@1:12+d"}},
		{"assignment", `ENV["X"] = "1"`, nil},
		{"comment", `# ENV["FAKE"]`, nil},
		{"block comment", "=begin\nENV['FAKE']\n=end\nENV['REAL']", []string{"REAL@4:6"}},
		{"string", `puts "ENV['FAKE']"`, nil},
		{"dynamic", `ENV[key]; ENV.fetch(name)`, []string{"dyn@1:1", "dyn@1:11"}},
	})
}

func TestPHP(t *testing.T) {
	runTable(t, discovery.LangPHP, []tc{
		{"getenv", "<?php\n$a = getenv('DB_HOST');", []string{"DB_HOST@2:14"}},
		{"getenv default", "<?php\n$a = getenv('DB_HOST') ?: 'localhost';", []string{"DB_HOST@2:14+d"}},
		{"env superglobal", "<?php\n$a = $_ENV['APP_KEY'] ?? null;", []string{"APP_KEY@2:13+d"}},
		{"server env var", "<?php\n$a = $_SERVER['APP_ENV'];", []string{"APP_ENV@2:16"}},
		{"server request data ignored", "<?php\n$a = $_SERVER['HTTP_HOST'] . $_SERVER['REQUEST_URI'] . $_SERVER['QUERY_STRING'];", nil},
		{"laravel env", "<?php\nreturn ['debug' => env('APP_DEBUG', false), 'k' => env('APP_KEY')];", []string{"APP_DEBUG@2:25+d", "APP_KEY@2:57+d"}},
		{"laravel env without default is optional", "<?php\nreturn ['url' => env('DB_URL')];", []string{"DB_URL@2:23+d"}},
		{"getenv comparison", "<?php\nif (getenv('APP_DEBUG') == 'true') {}", []string{"APP_DEBUG@2:13+d"}},
		{"method env ignored", "<?php\n$this->env('FAKE');", nil},
		{"comments", "<?php\n// getenv('FAKE')\n# getenv('FAKE2')\n/* env('FAKE3') */", nil},
		{"outside php tags", "<p>getenv('FAKE')</p>\n<?= getenv('REAL') ?>", []string{"REAL@2:13"}},
		{"dynamic", "<?php\ngetenv($k); $_ENV[$k];", []string{"dyn@2:1", "dyn@2:13"}},
	})
}

func TestRust(t *testing.T) {
	runTable(t, discovery.LangRust, []tc{
		{"env var", `let u = env::var("DATABASE_URL")?;`, []string{"DATABASE_URL@1:19"}},
		{"std env var default", `let p = std::env::var("PORT").unwrap_or("8080".into());`, []string{"PORT@1:24+d"}},
		{"ok is default", `let p = env::var("OPT").ok();`, []string{"OPT@1:19+d"}},
		{"env macro", `const V: &str = env!("CARGO_PKG_VERSION");`, []string{"CARGO_PKG_VERSION@1:23"}},
		{"option_env", `let v = option_env!("BUILD_SHA");`, []string{"BUILD_SHA@1:22+d"}},
		{"lifetime then usage", `fn f<'a>(x: &'a str) -> String { env::var("REAL").unwrap() }`, []string{"REAL@1:44"}},
		{"char literal quote", `let q = '"'; let v = env::var("REAL");`, []string{"REAL@1:32"}},
		{"raw string", `let s = r#"env::var("FAKE")"#; env::var("REAL")`, []string{"REAL@1:42"}},
		{"nested block comment", "/* a /* env::var(\"FAKE\") */ env::var(\"FAKE2\") */ env::var(\"REAL\")", []string{"REAL@1:60"}},
		{"dynamic", `env::var(key)`, []string{"dyn@1:1"}},
	})
}

func TestJVMAndDotNet(t *testing.T) {
	runTable(t, discovery.LangJava, []tc{
		{"getenv", `String u = System.getenv("DB_URL");`, []string{"DB_URL@1:27"}},
		{"getOrDefault", `System.getenv().getOrDefault("PORT", "8080")`, []string{"PORT@1:31+d"}},
		{"text block", "String s = \"\"\"\n  System.getenv(\"FAKE\")\n  \"\"\";", nil},
		{"dynamic", `System.getenv(name)`, []string{"dyn@1:1"}},
		{"no-arg getenv is not dynamic", `Map<String, String> m = System.getenv();`, nil},
	})
	runTable(t, discovery.LangKotlin, []tc{
		{"elvis default", `val p = System.getenv("PORT") ?: "8080"`, []string{"PORT@1:24+d"}},
	})
	runTable(t, discovery.LangCSharp, []tc{
		{"get", `var c = Environment.GetEnvironmentVariable("CONN");`, []string{"CONN@1:45"}},
		{"target and default", `var c = Environment.GetEnvironmentVariable("CONN", EnvironmentVariableTarget.Process) ?? "x";`, []string{"CONN@1:45+d"}},
		{"verbatim string with backslash", `var p = @"C:\"; var c = Environment.GetEnvironmentVariable("REAL");`, []string{"REAL@1:61"}},
		{"dynamic", `Environment.GetEnvironmentVariable(key)`, []string{"dyn@1:1"}},
	})
	runTable(t, discovery.LangElixir, []tc{
		{"get_env", `url = System.get_env("DATABASE_URL")`, []string{"DATABASE_URL@1:23"}},
		{"get_env default", `port = System.get_env("PORT", "4000")`, []string{"PORT@1:24+d"}},
		{"get_env or", `port = System.get_env("PORT") || "4000"`, []string{"PORT@1:24+d"}},
		{"fetch_env!", `key = System.fetch_env!("SECRET_KEY_BASE")`, []string{"SECRET_KEY_BASE@1:26"}},
		{"comment and heredoc", "# System.get_env(\"FAKE\")\n@doc \"\"\"\nSystem.get_env(\"FAKE2\")\n\"\"\"", nil},
	})
}

func TestShell(t *testing.T) {
	runTable(t, discovery.LangShell, []tc{
		{"braced", `echo "${API_URL}/health"`, []string{"API_URL@1:9"}},
		{"plain", `curl $API_URL`, []string{"API_URL@1:7"}},
		{"defaults", `: "${PORT:-8080}" "${HOST-localhost}" "${REQUIRED:?missing}" "${ALT:+x}"`,
			[]string{"PORT@1:6+d", "HOST@1:22+d", "REQUIRED@1:42", "ALT@1:65+d"}},
		{"single quotes are literal", `echo '$FAKE ${FAKE2}'`, nil},
		{"apostrophe inside double quotes", `echo "don't"; echo 'x $FAKE'; echo $REAL`, []string{"REAL@1:37"}},
		{"comment", "# uses $FAKE\necho $REAL # and $FAKE2", []string{"REAL@2:7"}},
		{"hash inside word is not a comment", `echo ${#ARR} x#y $REAL`, []string{"REAL@1:19"}},
		{"assigned in script", "NAME=demo\necho $NAME\nexport OUT=1; echo $OUT\nfor ITEM in a b; do echo $ITEM; done\nread -r ANSWER\necho $ANSWER", nil},
		{"self default assignment is an env read", `DEPLOY_ENV=${DEPLOY_ENV:-staging}`, []string{"DEPLOY_ENV@1:14+d"}},
		{"lowercase ignored", `echo $foo ${bar}`, nil},
		{"builtins ignored", `echo $RANDOM $LINENO $BASH_SOURCE $1 $@ $?`, nil},
		{"escaped dollar", `echo \$FAKE`, nil},
		{"indirect is dynamic", `echo ${!name}`, []string{"dyn@1:6"}},
		{"python shebang skipped", "#!/usr/bin/env python3\nprint('$X')\nx = $FAKE", nil},
		{"bash shebang ok", "#!/usr/bin/env bash\necho $REAL", []string{"REAL@2:7"}},
	})
}

func TestIsWellKnown(t *testing.T) {
	for _, n := range []string{"PATH", "HOME", "CI", "NODE_ENV", "GITHUB_SHA", "RUNNER_OS", "VERCEL_URL", "NETLIFY", "FLY_APP_NAME", "AWS_REGION", "KUBERNETES_SERVICE_HOST"} {
		if !IsWellKnown(n, false) {
			t.Errorf("%s should be well-known", n)
		}
	}
	for _, n := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "DATABASE_URL", "PORT"} {
		if IsWellKnown(n, false) {
			t.Errorf("%s should not be well-known", n)
		}
	}
	if !IsWellKnown("PORT", true) {
		t.Error("PORT with a default should be well-known")
	}
}

func TestNeverPanics(_ *testing.T) {
	inputs := []string{"", "`", "${", "/*", "'", "\"\"\"", "process.env[", "r#\"", "'\\", "<?php", "<script>", "=begin", "$'", "#!", "a/[/"}
	for lang := range languages {
		for _, in := range inputs {
			_ = scanFile(lang, "f", []byte(in))
			_ = scanFile(lang, "f", []byte(strings.Repeat(in, 50)))
		}
	}
}

// A detector that panics (a bug) must cost the run one file, not the whole
// process: the file is reported as unreadable and the others still scan.
func TestScanSurvivesPanickingDetector(t *testing.T) {
	const boom discovery.Language = "test-panic"
	languages[boom] = &language{syntax: jsSyntax, extra: []func(*fileCtx) []Usage{
		func(*fileCtx) []Usage { panic("CNRY-must-not-leak") },
	}}
	defer delete(languages, boom)

	dir := t.TempDir()
	for name, src := range map[string]string{"a.boom": "x", "b.js": "process.env.OK_NAME"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res := Scan(Options{Root: dir, Files: []discovery.File{
		{Path: "a.boom", Kind: discovery.KindSource, Language: boom, AppRoot: "."},
		{Path: "b.js", Kind: discovery.KindSource, Language: discovery.LangJavaScript, AppRoot: "."},
	}})
	if len(res.Unreadable) != 1 || res.Unreadable[0] != "a.boom" {
		t.Errorf("Unreadable = %v, want [a.boom]", res.Unreadable)
	}
	if len(res.Usages) != 1 || res.Usages[0].Variable != "OK_NAME" || res.Scanned != 1 {
		t.Errorf("the healthy file was not scanned: %+v", res)
	}
}

func TestComparisonsAreOptionalReads(t *testing.T) {
	runTable(t, discovery.LangJavaScript, []tc{
		{"strict equality", `export default process.env.ANALYZE === "true" ? a : b`, []string{"ANALYZE@1:28+d"}},
		{"not equal", `if (process.env.CI !== undefined) {}`, []string{"CI@1:17+d"}},
		{"loose", `if (process.env.DEBUG == "1") {}`, []string{"DEBUG@1:17+d"}},
		{"assignment is still not a read", `process.env.X = "1"`, nil},
		{"arrow is not a comparison", `const f = () => process.env.X; g(process.env.Y)`, []string{"X@1:29", "Y@1:46"}},
	})
	runTable(t, discovery.LangPython, []tc{
		{"getenv equals", `if os.getenv("DEBUG") == "1": pass`, []string{"DEBUG@1:15+d"}},
		{"environ.get not equal", `if os.environ.get("X") != "": pass`, []string{"X@1:20+d"}},
		{"getenv in", `if os.getenv("MODE") in ("a", "b"): pass`, []string{"MODE@1:15+d"}},
		{"subscript stays required: it raises KeyError", `if os.environ["X"] == "1": pass`, []string{"X@1:16"}},
		{"plain getenv", `x = os.getenv("X")`, []string{"X@1:16"}},
	})
	runTable(t, discovery.LangRuby, []tc{
		{"equals", `if ENV["RAILS_LOG_TO_STDOUT"] == "1" then end`, []string{"RAILS_LOG_TO_STDOUT@1:9+d"}},
		{"or-assign sets a default", `ENV["BUNDLE_GEMFILE"] ||= "x"`, []string{"BUNDLE_GEMFILE@1:6+d"}},
		{"plain assignment is a write", `ENV["X"] = "1"`, nil},
		{"plain read", `x = ENV["X"]`, []string{"X@1:10"}},
	})
}

func TestWholeEnvironmentReads(t *testing.T) {
	runTable(t, discovery.LangTypeScript, []tc{
		{"schema parse", `export const env = schema.parse(process.env);`, []string{"bulk@1:33"}},
		{"default parameter", `export function load(source: NodeJS.ProcessEnv = process.env): Env {}`, []string{"bulk@1:50"}},
		{"createEnv runtimeEnv", "createEnv({\n  runtimeEnv: process.env,\n})", []string{"bulk@2:15"}},
		{"cleanEnv argument", `const e = cleanEnv(process.env, { PORT: port() })`, []string{"bulk@1:20"}},
		{"second argument", `const e = validate(schema, process.env)`, []string{"bulk@1:28"}},
		// Pass-through to child processes says nothing about which names are read.
		{"spread is pass-through", `spawn(cmd, { env: { ...process.env, A: "1" } })`, nil},
		{"env property is pass-through", `spawn(cmd, { env: process.env })`, nil},
		{"keyed read is not bulk", `parse(process.env.PORT)`, []string{"PORT@1:19"}},
		{"keyed computed read is dynamic, not bulk", `read(process.env[name])`, []string{"dyn@1:6"}},
	})
}

func TestDjangoEnvironNeedsLibraryOrImport(t *testing.T) {
	src := []byte(`SECRET_KEY = env("DJANGO_SECRET_KEY")
DEBUG = env.bool("DJANGO_DEBUG", default=False)
`)
	// A settings module that only imports the configured env object.
	if got := render(scanFile(discovery.LangPython, "production.py", src)); len(got) != 0 {
		t.Errorf("without the library or an import, got %v", got)
	}
	got := render(scanFileWith(discovery.LangPython, "production.py", src, libDjangoEnviron))
	want := []string{"DJANGO_SECRET_KEY@1:19", "DJANGO_DEBUG@2:19+d"}
	if !slices.Equal(got, want) {
		t.Errorf("with django-environ in the app's dependencies: got %v, want %v", got, want)
	}
}

func TestDetectLibraries(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("with/requirements/base.txt", "Django==5.0\ndjango-environ==0.11.2\n")
	write("poetry/pyproject.toml", "[tool.poetry.dependencies]\ndjango_environ = \"^0.11\"\n")
	write("without/requirements.txt", "flask\n")
	files := []discovery.File{}
	for _, app := range []string{"with", "poetry", "without"} {
		files = append(files, discovery.File{Path: app + "/a.py", AppRoot: app, Language: discovery.LangPython})
	}
	got := detectLibraries(dir, files)
	if got["with"]&libDjangoEnviron == 0 || got["poetry"]&libDjangoEnviron == 0 || got["without"] != 0 {
		t.Errorf("detectLibraries = %v", got)
	}
}

// The detectors do not use \b (for speed); prevOK must keep rejecting matches
// that are glued to a longer name or are members of another object.
func TestMatchesGluedToIdentifiersAreRejected(t *testing.T) {
	runTable(t, discovery.LangJavaScript, []tc{
		{"prefix letter", `xprocess.env.FOO`, nil},
		{"prefix underscore", `_process.env.FOO`, nil},
		{"prefix digit", `1process.env.FOO`, nil},
		{"prefix dollar", `$process.env.FOO`, nil},
		{"member of another object", `foo.process.env.FOO`, nil},
		{"after a space is fine", `x = process.env.FOO`, []string{"FOO@1:17"}},
		{"after a paren is fine", `f(process.env.FOO)`, []string{"FOO@1:15"}},
		{"after a newline is fine", "x\nprocess.env.FOO", []string{"FOO@2:13"}},
		{"bulk read glued", `xprocess.env)`, nil},
	})
	runTable(t, discovery.LangPython, []tc{
		{"myos", `x = myos.environ["A"]`, nil},
		{"fakeos.getenv", `fakeos.getenv("A")`, nil},
		{"real", `x = os.getenv("A")`, []string{"A@1:16"}},
	})
	runTable(t, discovery.LangRuby, []tc{
		{"ENV as suffix", `MYENV["A"]`, nil},
		{"member", `Foo::ENV["A"]`, []string{"A@1:11"}}, // a constant lookup path, still the process ENV in practice
		{"real", `ENV["A"]`, []string{"A@1:6"}},
	})
	runTable(t, discovery.LangGo, []tc{
		{"suffix", `myos.Getenv("A")`, nil},
		{"real", `os.Getenv("A")`, []string{"A@1:12"}},
	})
	runTable(t, discovery.LangPHP, []tc{
		{"method call", "<?php $this->env('A');", nil},
		{"getenv suffix", "<?php mygetenv('A');", nil},
		{"real", "<?php env('A');", []string{"A@1:12+d"}},
	})
}
