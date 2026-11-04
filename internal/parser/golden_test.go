package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/testutil"
)

// TestGolden parses every testdata/parser/*.env file and compares the full
// result (entries, problems, directives) with <name>.golden.json.
// Regenerate with: go test ./internal/parser -run TestGolden -update
func TestGolden(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "parser", "*.env"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no golden inputs found")
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".env")
		t.Run(name, func(t *testing.T) {
			r, err := os.Open(in)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = r.Close() }()

			f, err := Parse(r, filepath.ToSlash(in))
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(f, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			testutil.Golden(t, filepath.Join("testdata", "parser", name+".golden.json"), got)
		})
	}
}
