package schema

import (
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }
func iptr(v int) *int        { return &v }

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		spec   Spec
		value  string
		ok     bool
		reason string // substring of the reason when !ok
	}{
		{"empty always passes", Spec{Type: TypeInt}, "", true, ""},
		{"string", Spec{}, "anything at all", true, ""},

		{"int ok", Spec{Type: TypeInt}, "42", true, ""},
		{"int negative", Spec{Type: TypeInt}, "-7", true, ""},
		{"int bad", Spec{Type: TypeInt}, "4.2", false, "expected an integer"},
		{"int min", Spec{Type: TypeInt, Min: f64(1)}, "0", false, "below the minimum 1"},
		{"int max", Spec{Type: TypeInt, Max: f64(64)}, "65", false, "above the maximum 64"},
		{"int range ok", Spec{Type: TypeInt, Min: f64(1), Max: f64(64)}, "64", true, ""},

		{"float ok", Spec{Type: TypeFloat}, "0.25", true, ""},
		{"float exp", Spec{Type: TypeFloat}, "1e3", true, ""},
		{"float bad", Spec{Type: TypeFloat}, "abc", false, "expected a number"},
		{"float nan", Spec{Type: TypeFloat}, "NaN", false, "expected a number"},
		{"float min", Spec{Type: TypeFloat, Min: f64(0.5)}, "0.4", false, "below the minimum 0.5"},

		{"bool true", Spec{Type: TypeBool}, "TRUE", true, ""},
		{"bool on", Spec{Type: TypeBool}, "on", true, ""},
		{"bool 0", Spec{Type: TypeBool}, "0", true, ""},
		{"bool bad", Spec{Type: TypeBool}, "maybe", false, "expected a boolean"},

		{"url ok", Spec{Type: TypeURL}, "https://api.example.com/v1", true, ""},
		{"url file path", Spec{Type: TypeURL}, "sqlite:///data/app.db", true, ""},
		{"url no scheme", Spec{Type: TypeURL}, "api.example.com", false, "expected a URL"},
		{"url host:port is not a url", Spec{Type: TypeURL}, "localhost:5432", false, "expected a URL"},
		{"url scheme ok", Spec{Type: TypeURL, Schemes: []string{"postgres", "postgresql"}}, "POSTGRESQL://db/app", true, ""},
		{"url scheme bad", Spec{Type: TypeURL, Schemes: []string{"postgres", "postgresql"}}, "mysql://db/app", false, "scheme postgres or postgresql"},
		{"url pattern", Spec{Type: TypeURL, Pattern: "^https://"}, "http://x.io", false, `does not match the pattern "^https://"`},

		{"port ok", Spec{Type: TypePort}, "8080", true, ""},
		{"port zero", Spec{Type: TypePort}, "0", false, "between 1 and 65535"},
		{"port too big", Spec{Type: TypePort}, "70000", false, "between 1 and 65535"},
		{"port min", Spec{Type: TypePort, Min: f64(1024)}, "80", false, "below the minimum 1024"},

		{"email ok", Spec{Type: TypeEmail}, "ops@example.com", true, ""},
		{"email bad", Spec{Type: TypeEmail}, "ops@localhost", false, "expected an email"},
		{"email spaces", Spec{Type: TypeEmail}, "a b@c.de", false, "expected an email"},

		{"hostname ok", Spec{Type: TypeHostname}, "db.internal", true, ""},
		{"hostname localhost", Spec{Type: TypeHostname}, "localhost", true, ""},
		{"hostname ipv4", Spec{Type: TypeHostname}, "10.0.0.1", true, ""},
		{"hostname ipv6", Spec{Type: TypeHostname}, "::1", true, ""},
		{"hostname bad", Spec{Type: TypeHostname}, "-bad-.host", false, "expected a hostname"},
		{"hostname with scheme", Spec{Type: TypeHostname}, "https://x.io", false, "expected a hostname"},

		{"duration ok", Spec{Type: TypeDuration}, "1h30m", true, ""},
		{"duration ms", Spec{Type: TypeDuration}, "300ms", true, ""},
		{"duration bare number", Spec{Type: TypeDuration}, "30", false, "expected a duration"},

		{"enum ok", Spec{Type: TypeEnum, Values: []string{"debug", "info"}}, "info", true, ""},
		{"enum case-sensitive", Spec{Type: TypeEnum, Values: []string{"debug", "info"}}, "INFO", false, "expected one of debug or info"},

		{"json ok", Spec{Type: TypeJSON}, `{"a": [1, 2]}`, true, ""},
		{"json bad", Spec{Type: TypeJSON}, `{a: 1}`, false, "expected valid JSON"},

		{"regex ok", Spec{Type: TypeRegex}, `^[a-z]+$`, true, ""},
		{"regex bad", Spec{Type: TypeRegex}, `([`, false, "expected a valid regular expression"},

		{"min length", Spec{MinLength: iptr(3)}, "ab", false, "shorter than 3"},
		{"max length runes", Spec{MaxLength: iptr(2)}, "日本", true, ""},
		{"max length", Spec{MaxLength: iptr(2)}, "abc", false, "longer than 2"},
		{"pattern string", Spec{Pattern: `^[a-z0-9]{8}$`}, "abcd1234", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, ok := tt.spec.Check(tt.value)
			if ok != tt.ok {
				t.Fatalf("Check(%q) ok = %v (reason %q), want %v", tt.value, ok, reason, tt.ok)
			}
			if !ok {
				if !strings.Contains(reason, tt.reason) {
					t.Errorf("reason %q does not contain %q", reason, tt.reason)
				}
				if len(tt.value) >= 3 && strings.Contains(reason, tt.value) {
					t.Errorf("reason %q echoes the value", reason)
				}
			}
		})
	}
}

func TestParseBool(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{{"true", true}, {"True", true}, {"  yes\t", true}, {"1", true}, {"ON", true},
		{"false", false}, {"No", false}, {"0", false}, {"off", false}} {
		if got, ok := ParseBool(tc.in); !ok || got != tc.want {
			t.Errorf("ParseBool(%q) = %v, %v", tc.in, got, ok)
		}
	}
	for _, in := range []string{"", "maybe", "2", "y", "enabled"} {
		if _, ok := ParseBool(in); ok {
			t.Errorf("ParseBool(%q) accepted", in)
		}
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		spec Spec
		a, b string
		want bool
	}{
		{Spec{Type: TypeBool}, "off", "false", true},
		{Spec{Type: TypeBool}, "1", "true", true},
		{Spec{Type: TypeBool}, "yes", "false", false},
		{Spec{Type: TypeInt}, "007", "7", true},
		{Spec{Type: TypePort}, "80", "8080", false},
		{Spec{}, "a", "A", false},
	}
	for _, tt := range tests {
		if got := tt.spec.Equal(tt.a, tt.b); got != tt.want {
			t.Errorf("%s Equal(%q, %q) = %v", tt.spec.typ(), tt.a, tt.b, got)
		}
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name  string
		spec  Spec
		field string // "" = valid
	}{
		{"valid int", Spec{Type: TypeInt, Min: f64(1), Max: f64(2)}, ""},
		{"unknown type", Spec{Type: "integer"}, "type"},
		{"enum without values", Spec{Type: TypeEnum}, "type"},
		{"values without enum", Spec{Values: []string{"a"}}, "values"},
		{"schemes without url", Spec{Type: TypeString, Schemes: []string{"https"}}, "schemes"},
		{"min on bool", Spec{Type: TypeBool, Min: f64(1)}, "min"},
		{"min > max", Spec{Type: TypeFloat, Min: f64(2), Max: f64(1)}, "min"},
		{"negative length", Spec{MinLength: iptr(-1)}, "min_length"},
		{"lengths inverted", Spec{MinLength: iptr(5), MaxLength: iptr(2)}, "min_length"},
		{"bad pattern", Spec{Pattern: "("}, "pattern"},
	}
	for _, tt := range tests {
		errs := ValidateSpec(tt.spec)
		switch {
		case tt.field == "" && len(errs) > 0:
			t.Errorf("%s: unexpected %v", tt.name, errs)
		case tt.field != "" && (len(errs) == 0 || errs[0].Field != tt.field):
			t.Errorf("%s: errors %v, want field %s", tt.name, errs, tt.field)
		}
	}
}

func TestDescribe(t *testing.T) {
	tests := map[string]Spec{
		"an integer between 1 and 64":                 {Type: TypeInt, Min: f64(1), Max: f64(64)},
		"a URL with scheme postgres or postgresql":    {Type: TypeURL, Schemes: []string{"postgres", "postgresql"}},
		`a URL matching "^https://"`:                  {Type: TypeURL, Pattern: "^https://"},
		"one of debug, info, warn or error":           {Type: TypeEnum, Values: []string{"debug", "info", "warn", "error"}},
		"a port (1-65535)":                            {Type: TypePort},
		"a boolean (true/false, 1/0, yes/no, on/off)": {Type: TypeBool},
		"a string": {},
	}
	for want, spec := range tests {
		if got := spec.Describe(); got != want {
			t.Errorf("Describe = %q, want %q", got, want)
		}
	}
}
