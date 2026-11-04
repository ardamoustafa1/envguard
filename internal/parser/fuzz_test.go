package parser

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"unicode/utf8"
)

func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		"K=v",
		"export K=v # c",
		"K=\"a\\n${B:-c}\"\r\nX='y'\rZ=`m\nn`",
		"\xef\xbb\xbfK=日本 $A",
		"K=\"unterminated\nX=1",
		"# envguard-ignore EG001,eg002 -- r\nK=1",
		"K=${A:-${B:-${C}}}",
		"K=\"\\",
		"=\n1=\nK\n",
		"K=\"a\\\r\nb\"",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	files, _ := filepath.Glob(filepath.Join("testdata", "parser", "*.env"))
	for _, path := range files {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		f1, err := Parse(bytes.NewReader(data), "fuzz.env")
		if err != nil {
			t.Fatalf("Parse returned error for in-memory input: %v", err)
		}
		f2, err := Parse(bytes.NewReader(data), "fuzz.env")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(f1, f2) {
			t.Fatal("parsing is not deterministic")
		}
		if len(data) > MaxFileSize {
			return
		}

		check := newPosChecker(data)
		for i := range f1.Entries {
			e := &f1.Entries[i]
			check.pos(t, "KeyPos", e.KeyPos)
			check.pos(t, "ValuePos", e.ValuePos)
			for _, r := range e.References {
				check.pos(t, "Reference", r.Pos)
				if !bytes.HasPrefix(data[r.Pos.Offset:], []byte("$")) {
					t.Errorf("reference %q does not point at '$'", r.Name)
				}
			}
			if !bytes.HasPrefix(data[e.KeyPos.Offset:], []byte(e.Key)) {
				t.Errorf("KeyPos of %q does not point at the key", e.Key)
			}
			if !bytes.HasPrefix(data[e.ValuePos.Offset:], []byte(e.RawValue)) {
				t.Errorf("ValuePos of %q does not point at RawValue", e.Key)
			}
			if e.Key == "" {
				t.Error("entry with empty key")
			}
		}
		for _, p := range f1.Problems {
			check.pos(t, "Problem", p.Pos)
			if p.Message == "" || p.Hint == "" {
				t.Errorf("problem %s without message or hint", p.Kind)
			}
		}
		for _, d := range f1.Directives {
			check.pos(t, "Directive", d.Pos)
		}
		if !sort.SliceIsSorted(f1.Problems, func(i, j int) bool {
			return f1.Problems[i].Pos.Offset < f1.Problems[j].Pos.Offset
		}) {
			t.Error("problems not sorted by offset")
		}
	})
}

// posChecker independently recomputes line and column for every offset in
// one linear pass, so each check is O(1) and the harness stays fast on large
// inputs.
type posChecker struct {
	bom   int
	end   int   // len(data)
	lines []int // line number for each offset relative to the BOM-less source (len(src)+1 entries)
	cols  []int // column number for each offset, same indexing
}

func newPosChecker(data []byte) posChecker {
	bom := 0
	if bytes.HasPrefix(data, []byte(utf8BOM)) {
		bom = len(utf8BOM)
	}
	src := data[bom:]
	c := posChecker{bom: bom, end: len(data), lines: make([]int, len(src)+1), cols: make([]int, len(src)+1)}
	// Decode rune by rune. An offset k bytes into a rune gets column col+k,
	// matching utf8.RuneCount on a prefix that ends mid-rune.
	line, col := 1, 1
	for i := 0; i < len(src); {
		_, size := utf8.DecodeRune(src[i:])
		for k := range size {
			c.lines[i+k], c.cols[i+k] = line, col+k
		}
		col++
		if src[i] == '\n' || (src[i] == '\r' && (i+1 >= len(src) || src[i+1] != '\n')) {
			line, col = line+1, 1
		}
		i += size
	}
	c.lines[len(src)], c.cols[len(src)] = line, col
	return c
}

func (c posChecker) pos(t *testing.T, what string, p Pos) {
	t.Helper()
	if p.Offset < c.bom || p.Offset > c.end {
		t.Fatalf("%s offset %d out of bounds [%d,%d]", what, p.Offset, c.bom, c.end)
	}
	off := p.Offset - c.bom
	if p.Line != c.lines[off] {
		t.Fatalf("%s line = %d, want %d (offset %d)", what, p.Line, c.lines[off], p.Offset)
	}
	if p.Column != c.cols[off] {
		t.Fatalf("%s column = %d, want %d (line %d, offset %d)", what, p.Column, c.cols[off], p.Line, p.Offset)
	}
}
