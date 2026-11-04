package findings

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSeverity(t *testing.T) {
	for _, s := range []Severity{Info, Warning, Error} {
		p, err := ParseSeverity(s.String())
		if err != nil || p != s {
			t.Errorf("round trip %v: %v %v", s, p, err)
		}
	}
	if p, err := ParseSeverity(" WARNING "); err != nil || p != Warning {
		t.Errorf("case-insensitive parse failed: %v %v", p, err)
	}
	if _, err := ParseSeverity("warn"); err == nil {
		t.Error("want error for warn")
	}
	if Info >= Warning || Warning >= Error || !Error.AtLeast(Warning) || Info.AtLeast(Warning) {
		t.Error("ordering broken")
	}
	b, err := json.Marshal(struct{ S Severity }{Warning})
	if err != nil || string(b) != `{"S":"warning"}` {
		t.Errorf("json = %s %v", b, err)
	}
	var v struct{ S Severity }
	if err := json.Unmarshal([]byte(`{"S":"error"}`), &v); err != nil || v.S != Error {
		t.Errorf("unmarshal = %v %v", v.S, err)
	}
	if _, err := json.Marshal(Severity(0)); err == nil {
		t.Error("zero severity must not marshal")
	}
}

func f(rule, file string, line, col int, variable string) Finding {
	return Finding{RuleID: rule, File: file, Line: line, Column: col, Variable: variable, Message: rule + " " + variable, Severity: Error}
}

func TestSortAndDedupe(t *testing.T) {
	in := []Finding{
		f("EG002", "b/.env", 1, 1, "A"),
		f("EG001", "a/.env", 10, 1, "B"),
		f("EG001", "a/.env", 2, 5, "C"),
		f("EG006", "a/.env", 2, 1, "A"),
		f("EG001", "a/.env", 2, 1, "A"),
		f("EG001", "a/.env", 2, 1, "A"), // duplicate
		f("EG001", "a/.env", 0, 0, "Z"),
	}
	got := Dedupe(in)
	var order []string
	for _, x := range got {
		order = append(order, x.Location().String()+" "+x.RuleID+" "+x.Variable)
	}
	want := []string{
		"a/.env EG001 Z",
		"a/.env:2:1 EG001 A",
		"a/.env:2:1 EG006 A",
		"a/.env:2:5 EG001 C",
		"a/.env:10:1 EG001 B",
		"b/.env:1:1 EG002 A",
	}
	if !slices.Equal(order, want) {
		t.Errorf("order =\n%v\nwant\n%v", order, want)
	}
}

func TestApply(t *testing.T) {
	in := []Finding{
		f("EG001", "apps/api/.env", 3, 1, "DB_URL"),
		f("EG002", "apps/api/.env", 4, 1, "NEXT_PUBLIC_X"),
		f("EG002", "apps/web/.env", 4, 1, "NEXT_PUBLIC_Y"),
		f("EG004", "apps/web/.env", 5, 1, "UNUSED"),
		f("EG006", "scripts/.env", 1, 1, "A"),
		f("EG007", ".env", 9, 1, ""),
		{RuleID: "EG001", File: ".env", Variable: "TPL", Severity: Error, Related: []Location{{File: ".env.example", Line: 7}}},
	}
	s := Suppression{
		Overrides: map[string]string{"EG004": "off", "EG001": "warning"},
		Ignores: []Ignore{
			{Variable: "NEXT_PUBLIC_*"},
			{Rule: "EG006", Path: "scripts/**"},
			{Rule: "EG007", Variable: "X"}, // never matches findings without a variable
		},
		Inline: func(rule string, loc Location) bool {
			return rule == "EG001" && loc.File == ".env.example" && loc.Line == 7
		},
	}
	res := s.Apply(in)
	if res.Suppressed != 4 {
		t.Errorf("Suppressed = %d, want 4", res.Suppressed)
	}
	var got []string
	for _, x := range res.Findings {
		got = append(got, x.RuleID+":"+x.Variable+":"+x.Severity.String())
	}
	want := []string{"EG007::error", "EG001:DB_URL:warning"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !s.RuleOff("EG004") || s.RuleOff("EG001") {
		t.Error("RuleOff wrong")
	}
}

func TestMatchName(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"A", "A", true}, {"A", "AB", false}, {"NEXT_*", "NEXT_PUBLIC", true}, {"*_URL", "DB_URL", true},
		{"*", "X", true}, {"A*B", "AxxB", true}, {"A*B", "AxxC", false}, {"a.b*", "a.bc", true}, {"a.b*", "axbc", false},
	}
	for _, tt := range tests {
		if got := MatchName(tt.pattern, tt.name); got != tt.want {
			t.Errorf("MatchName(%q, %q) = %v", tt.pattern, tt.name, got)
		}
	}
	if !MatchPath(".env", "apps/api/.env") || MatchPath("apps/*", "apps/a/b") || !MatchPath("apps/**", "apps/a/b") {
		t.Error("MatchPath wrong")
	}
}

func TestCount(t *testing.T) {
	fs := []Finding{{Severity: Error}, {Severity: Warning}, {Severity: Warning}, {Severity: Info}}
	if c := Count(fs); c != (Counts{Error: 1, Warning: 2, Info: 1}) {
		t.Errorf("Count = %+v", c)
	}
	if !AnyAtLeast(fs, Error) || AnyAtLeast(fs[1:], Error) {
		t.Error("AnyAtLeast wrong")
	}
}
