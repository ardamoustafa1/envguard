package compose

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var nameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// FuzzParse feeds arbitrary bytes to the Compose parser and its
// interpolation scanner. It must never panic or hang, be deterministic, and
// report only well-formed names at positions inside the file.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"",
		"services:\n  api:\n    environment:\n      A: ${B:-c}\n      D: $E\n      F: $$G\n",
		"services:\n  a:\n    environment:\n      - X=${Y?err}\n      - Z\n    env_file:\n      - .env\n      - path: ./o.env\n        required: false\n",
		"x-base: &base\n  environment:\n    A: ${B}\nservices:\n  a:\n    <<: *base\n",
		"a: &a [*a]\n",
		"services: [\n",
		"${A:-${B:-${C}}}\n$\n${\n${A\n$$$A${B}",
		"services:\n  \"\xff\":\n    environment: {\"A\xc3\": ${\xe2\x82}}\n",
		"a: &a {b: *a}\nc: *a\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		f1 := Parse(data, "dir/compose.yaml")
		if !reflect.DeepEqual(f1, Parse(data, "dir/compose.yaml")) {
			t.Fatal("parsing is not deterministic")
		}
		lines := yamlLines(data)
		check := func(what string, line, col int) {
			if line < 1 || line > lines+1 || col < 1 {
				t.Fatalf("%s position %d:%d outside a %d-line file", what, line, col, lines)
			}
		}
		for _, it := range f1.Interpolations {
			if !nameRE.MatchString(it.Variable) {
				t.Fatalf("interpolation of malformed name %q", it.Variable)
			}
			check("interpolation", it.Line, it.Column)
			if it.HasDefault && it.Required {
				t.Fatal("interpolation both has a default and is required")
			}
		}
		for _, s := range f1.Services {
			check("service", s.Line, s.Column)
			for _, e := range s.Environment {
				check("environment", e.Line, e.Column)
			}
			for _, e := range s.EnvFiles {
				check("env_file", e.Line, e.Column)
				if strings.HasPrefix(e.Path, "/") || strings.HasPrefix(e.Path, "../") || strings.Contains(e.Path, "/../") {
					t.Fatalf("env_file resolved outside the root: %q", e.Path)
				}
			}
		}
		if f1.Problem != nil && (len(f1.Services) > 0 || len(f1.Interpolations) > 0) {
			t.Fatal("results reported for an invalid YAML document")
		}
	})
}

// yamlLines counts lines the way YAML does: LF, CR and CRLF all end a line.
func yamlLines(data []byte) int {
	n := 1
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' || (data[i] == '\r' && (i+1 >= len(data) || data[i+1] != '\n')) {
			n++
		}
	}
	return n
}
