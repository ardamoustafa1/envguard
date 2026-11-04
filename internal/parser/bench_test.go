package parser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// benchInput builds a realistic 5,000-line env file mixing every syntax form.
func benchInput() []byte {
	var b strings.Builder
	for i := range 5000 {
		switch i % 10 {
		case 0:
			fmt.Fprintf(&b, "# section %d\n", i)
		case 1:
			fmt.Fprintf(&b, "export APP_%d=value-%d\n", i, i)
		case 2:
			fmt.Fprintf(&b, "URL_%d=https://example.com/p?a=%d#frag # comment\n", i, i)
		case 3:
			fmt.Fprintf(&b, "DQ_%d=\"hello\\nworld ${HOST_%d:-localhost}:$PORT\"\n", i, i)
		case 4:
			fmt.Fprintf(&b, "SQ_%d='{\"json\": [1, 2, 3]}'\n", i)
		case 5:
			fmt.Fprintf(&b, "B64_%d=ZGVtby12YWx1ZS0xMjM0NTY3ODkw==\n", i)
		case 6:
			fmt.Fprintf(&b, "UNI_%d=\"héllo wörld ✓ 日本語\"\n", i)
		case 7:
			fmt.Fprintf(&b, "EMPTY_%d=\n", i)
		case 8:
			fmt.Fprintf(&b, "# envguard-ignore EG002\n")
		default:
			fmt.Fprintf(&b, "  SPACED_%d = value_%d\r\n", i, i)
		}
	}
	return []byte(b.String())
}

func BenchmarkParse5000Lines(b *testing.B) {
	data := benchInput()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse(bytes.NewReader(data), "bench.env"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestBenchInputParsesCleanly(t *testing.T) {
	f, err := Parse(bytes.NewReader(benchInput()), "bench.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Problems) != 0 {
		t.Fatalf("bench input has problems: %+v", f.Problems[0])
	}
	if len(f.Entries) != 4000 {
		t.Fatalf("entries = %d, want 4000", len(f.Entries))
	}
}
