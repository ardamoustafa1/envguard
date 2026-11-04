package config

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/schema"
)

// FuzzParse feeds arbitrary bytes to the strict config loader. It must never
// panic or hang, be deterministic, fail only with *Error carrying located
// one-line problems, and when it succeeds produce a configuration whose
// every variable passes schema validation.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"",
		"version: 1\n",
		"version: 2\n",
		"version: 1\nfail_on: warn\nexlude: [a]\nrules: {EG001: off, EG99: x}\n",
		"version: 1\nvariables:\n  A: {type: int, min: 5, max: 1}\n  B: {type: enum}\n  C: {type: url, schemes: [https], pattern: '('}\n  D: {type: port, default: x, production: {must_be: a, must_not_be: a}}\n",
		"version: 1\nenvironments:\n  Bad Name: {files: ['**/*.env']}\nignore:\n  - rule: EG001\nscanner: {languages: [cobol], ignore_variables: [A]}\nprofiles: [nope]\n",
		"version: 1\nversion: 1\n",
		"a: &a [*a]\nversion: 1\n",
		"version: 1\nvariables:\n  \"\xff\": {type: \"\xe2\x82\"}\n",
		"\t\tversion: 1\n",
		"version: [1\n",
		"version: 1\ninclude: [\"[\", \"**/\\\\\"]\nexclude: [\"{a\"]\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		f1, err1 := Parse(data, "fuzz.yaml")
		f2, err2 := Parse(data, "fuzz.yaml")
		if (err1 == nil) != (err2 == nil) || (err1 != nil && err1.Error() != err2.Error()) || !reflect.DeepEqual(f1, f2) {
			t.Fatal("parsing is not deterministic")
		}
		if err1 != nil {
			var ce *Error
			if !errors.As(err1, &ce) || !errors.Is(err1, ErrInvalid) || len(ce.Problems) == 0 {
				t.Fatalf("error is not a *config.Error with problems: %v", err1)
			}
			for _, p := range ce.Problems {
				if p.Message == "" || strings.ContainsAny(p.Message, "\r\n") || p.Line < 0 || p.Column < 0 {
					t.Fatalf("bad problem %+v", p)
				}
			}
			return
		}
		if f1.Version != 1 {
			t.Fatalf("accepted version %d", f1.Version)
		}
		for name, v := range f1.Variables {
			if problems := schema.ValidateSpec(v.Spec()); len(problems) > 0 {
				t.Fatalf("variable %s accepted with an invalid spec: %v", name, problems)
			}
		}
	})
}
