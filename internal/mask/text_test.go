package mask

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPrintable(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain.env", "plain.env"},
		{"ünï cødé/日本語/.env", "ünï cødé/日本語/.env"},
		{"tab\tand\nnewline", "tab\tand\nnewline"},
		{".env.\x1b[2J\x1b]0;pwned\x07", `.env.\x1b[2J\x1b]0;pwned\x07`},
		{"cr\rlf", `cr\x0dlf`},
		{"del\x7f", `del\x7f`},
		{"c1\u0085\u009b", `c1\u0085\u009b`},
		{"evil\u202etxt.exe", `evil\u202etxt.exe`},
		{"iso\u2066x\u2069", `iso\u2066x\u2069`},
		{"zero\u200bwidth\ufeff", `zero\u200bwidth\ufeff`},
		{"sep\u2028\u2029", `sep\u2028\u2029`},
		{"bad\xff\xc3(", `bad\xff\xc3(`},
		{"nul\x00", `nul\x00`},
		{"", ""},
	}
	for _, tt := range tests {
		got := Printable(tt.in)
		if got != tt.want {
			t.Errorf("Printable(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("Printable(%q) is not valid UTF-8", tt.in)
		}
		if Printable(got) != got {
			t.Errorf("Printable is not idempotent on %q", tt.in)
		}
	}
}

func TestClip(t *testing.T) {
	long := strings.Repeat("x", 500_000)
	tests := []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"elevenchars", 10, "elevencha…"},
		{"日本語日本語日本語日本語", 5, "日本語日…"},
		{long, 5, "xxxx…"},
		{"abc", 1, "abc"}, // a limit below 2 cannot clip meaningfully
	}
	for _, tt := range tests {
		if got := Clip(tt.in, tt.limit); got != tt.want {
			t.Errorf("Clip(%.12q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
		}
	}
}
