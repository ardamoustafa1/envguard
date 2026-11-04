package workflow

import (
	"reflect"
	"testing"
)

// FuzzParse feeds arbitrary bytes to the workflow parser: no panic or hang,
// deterministic, and positions inside the file.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"",
		"on: push\nenv:\n  A: 1\njobs:\n  b:\n    env:\n      C: ${{ secrets.D }}\n    steps:\n      - run: echo ${{ vars.E }} ${{ env.F }}\n        if: secrets.G != ''\n        env:\n          H: ${{ secrets['I'] }}\n",
		"jobs: [\n",
		"a: &a [*a]\n",
		"x: ${{ secrets.\n${{ }}\n${{ secrets..A }}",
		"jobs:\n  \xff:\n    steps: [{run: ${{ secrets.\xe2\x82 }}}]\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		f1 := Parse(data, ".github/workflows/x.yml")
		if !reflect.DeepEqual(f1, Parse(data, ".github/workflows/x.yml")) {
			t.Fatal("parsing is not deterministic")
		}
		lines := yamlLines(data)
		for _, r := range f1.Refs {
			if r.Name == "" || r.Line < 1 || r.Line > lines+1 || r.Column < 1 {
				t.Fatalf("bad reference %+v in a %d-line file", r, lines)
			}
			if r.Context != "secrets" && r.Context != "vars" && r.Context != "env" {
				t.Fatalf("unknown context %q", r.Context)
			}
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
