package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Types supported by the schema.
const (
	TypeString   = "string"
	TypeInt      = "int"
	TypeFloat    = "float"
	TypeBool     = "bool"
	TypeURL      = "url"
	TypePort     = "port"
	TypeEmail    = "email"
	TypeHostname = "hostname"
	TypeDuration = "duration"
	TypeEnum     = "enum"
	TypeJSON     = "json"
	TypeRegex    = "regex"
)

// Types returns every supported type, in documentation order.
func Types() []string {
	return []string{TypeString, TypeInt, TypeFloat, TypeBool, TypeURL, TypePort, TypeEmail, TypeHostname,
		TypeDuration, TypeEnum, TypeJSON, TypeRegex}
}

// Spec is the value constraint of one variable. The zero value accepts any
// string.
type Spec struct {
	Type      string // "" = string
	Values    []string
	Schemes   []string
	Pattern   string
	Min, Max  *float64
	MinLength *int
	MaxLength *int
}

// SpecError is a problem with a Spec itself; Field names the offending key.
type SpecError struct {
	Field   string
	Message string
}

// ValidateSpec checks a Spec for internal consistency: known type, enum
// with values, values/schemes/min/max only where they apply, min <= max,
// non-negative lengths, compilable pattern.
func ValidateSpec(s Spec) []SpecError {
	var errs []SpecError
	add := func(field, format string, args ...any) {
		errs = append(errs, SpecError{Field: field, Message: fmt.Sprintf(format, args...)})
	}
	t := s.typ()
	if !slices.Contains(Types(), t) {
		add("type", "unknown type; allowed: %s", strings.Join(Types(), ", "))
		return errs
	}
	if t == TypeEnum && len(s.Values) == 0 {
		add("type", `type enum requires a non-empty "values" list`)
	}
	if t != TypeEnum && len(s.Values) > 0 {
		add("values", `"values" only applies to type enum`)
	}
	if t != TypeURL && len(s.Schemes) > 0 {
		add("schemes", `"schemes" only applies to type url`)
	}
	if (s.Min != nil || s.Max != nil) && !s.numeric() {
		add("min", `"min" and "max" only apply to types int, float and port`)
	}
	for _, f := range []*float64{s.Min, s.Max} {
		if f != nil && (math.IsNaN(*f) || math.IsInf(*f, 0)) {
			add("min", `"min" and "max" must be finite numbers`)
		}
	}
	if s.Min != nil && s.Max != nil && *s.Min > *s.Max {
		add("min", `"min" is greater than "max"`)
	}
	if s.MinLength != nil && *s.MinLength < 0 {
		add("min_length", `"min_length" must not be negative`)
	}
	if s.MaxLength != nil && *s.MaxLength < 0 {
		add("max_length", `"max_length" must not be negative`)
	}
	if s.MinLength != nil && s.MaxLength != nil && *s.MinLength > *s.MaxLength {
		add("min_length", `"min_length" is greater than "max_length"`)
	}
	if s.Pattern != "" {
		if _, err := regexp.Compile(s.Pattern); err != nil {
			add("pattern", "invalid regular expression (Go RE2 syntax)")
		}
	}
	return errs
}

func (s Spec) typ() string {
	if s.Type == "" {
		return TypeString
	}
	return s.Type
}

func (s Spec) numeric() bool {
	t := s.typ()
	return t == TypeInt || t == TypeFloat || t == TypePort
}

// Describe returns a short human description of what the Spec accepts,
// e.g. "an int between 1 and 64" or "a url with scheme postgres or postgresql".
func (s Spec) Describe() string {
	var d string
	switch t := s.typ(); t {
	case TypeInt:
		d = "an integer"
	case TypeFloat:
		d = "a number"
	case TypeBool:
		d = "a boolean (true/false, 1/0, yes/no, on/off)"
	case TypeURL:
		d = "a URL"
		if len(s.Schemes) > 0 {
			d += " with scheme " + orList(s.Schemes)
		}
	case TypePort:
		d = "a port (1-65535)"
	case TypeEmail:
		d = "an email address"
	case TypeHostname:
		d = "a hostname or IP address"
	case TypeDuration:
		d = "a duration such as 300ms, 30s or 1h30m"
	case TypeEnum:
		d = "one of " + orList(s.Values)
	case TypeJSON:
		d = "valid JSON"
	case TypeRegex:
		d = "a valid regular expression"
	default:
		d = "a string"
	}
	switch {
	case s.Min != nil && s.Max != nil:
		d += fmt.Sprintf(" between %s and %s", num(*s.Min), num(*s.Max))
	case s.Min != nil:
		d += " of at least " + num(*s.Min)
	case s.Max != nil:
		d += " of at most " + num(*s.Max)
	}
	if s.Pattern != "" {
		d += fmt.Sprintf(" matching %q", s.Pattern)
	}
	return d
}

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func orList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
	}
}

// Check validates a non-empty value. It returns ok, or a reason that never
// contains the value itself ("expected an integer", "is below the minimum 1",
// "does not match the pattern ^https://"). Empty values are the business of
// required/empty checks, not of Check, and always pass.
func (s Spec) Check(value string) (reason string, ok bool) {
	if value == "" {
		return "", true
	}
	var n float64
	hasNum := false
	switch s.typ() {
	case TypeInt:
		i, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return "expected an integer", false
		}
		n, hasNum = float64(i), true
	case TypeFloat:
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "expected a number", false
		}
		n, hasNum = f, true
	case TypePort:
		i, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil || i < 1 || i > 65535 {
			return "expected a port number between 1 and 65535", false
		}
		n, hasNum = float64(i), true
	case TypeBool:
		if _, ok := ParseBool(value); !ok {
			return "expected a boolean (true/false, 1/0, yes/no, on/off)", false
		}
	case TypeURL:
		if r, ok := checkURL(value, s.Schemes); !ok {
			return r, false
		}
	case TypeEmail:
		if !emailRE.MatchString(value) {
			return "expected an email address", false
		}
	case TypeHostname:
		if !IsHostname(value) {
			return "expected a hostname or IP address", false
		}
	case TypeDuration:
		if _, err := time.ParseDuration(strings.TrimSpace(value)); err != nil {
			return "expected a duration such as 300ms, 30s or 1h30m", false
		}
	case TypeEnum:
		if !slices.Contains(s.Values, value) {
			return "expected one of " + orList(s.Values), false
		}
	case TypeJSON:
		if !json.Valid([]byte(value)) {
			return "expected valid JSON", false
		}
	case TypeRegex:
		if _, err := regexp.Compile(value); err != nil {
			return "expected a valid regular expression", false
		}
	}
	if hasNum {
		if s.Min != nil && n < *s.Min {
			return "is below the minimum " + num(*s.Min), false
		}
		if s.Max != nil && n > *s.Max {
			return "is above the maximum " + num(*s.Max), false
		}
	}
	length := utf8.RuneCountInString(value)
	if s.MinLength != nil && length < *s.MinLength {
		return fmt.Sprintf("is shorter than %d characters", *s.MinLength), false
	}
	if s.MaxLength != nil && length > *s.MaxLength {
		return fmt.Sprintf("is longer than %d characters", *s.MaxLength), false
	}
	if s.Pattern != "" {
		re, err := regexp.Compile(s.Pattern)
		if err == nil && !re.MatchString(value) {
			return fmt.Sprintf("does not match the pattern %q", s.Pattern), false
		}
	}
	return "", true
}

// ParseBool accepts true/false, 1/0, yes/no and on/off, case-insensitively
// and ignoring surrounding whitespace.
func ParseBool(s string) (value, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return false, true
	}
	return false, false
}

var emailRE = regexp.MustCompile(`^[^@\s<>]+@[^@\s<>]+\.[^@\s<>.]+$`)

var labelRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// IsHostname reports whether s is an RFC 1123 hostname or an IP address.
func IsHostname(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(s, "."), ".") {
		if !labelRE.MatchString(label) {
			return false
		}
	}
	return true
}

func checkURL(value string, schemes []string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Scheme == "" || u.Opaque != "" || (u.Host == "" && u.Path == "") {
		return "expected a URL such as https://host/path", false
	}
	if len(schemes) > 0 && !slices.ContainsFunc(schemes, func(s string) bool { return strings.EqualFold(s, u.Scheme) }) {
		return "expected a URL with scheme " + orList(schemes), false
	}
	return "", true
}

// Equal compares two values the way the type sees them: booleans by
// meaning (on == true), numbers by value, everything else as strings.
func (s Spec) Equal(a, b string) bool {
	switch s.typ() {
	case TypeBool:
		x, ok1 := ParseBool(a)
		y, ok2 := ParseBool(b)
		if ok1 && ok2 {
			return x == y
		}
	case TypeInt, TypeFloat, TypePort:
		x, err1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
		y, err2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if err1 == nil && err2 == nil {
			return x == y
		}
	}
	return a == b
}
