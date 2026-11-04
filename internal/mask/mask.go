package mask

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxPlainLength is the longest value (in characters) Display may show.
const MaxPlainLength = 24

// sensitiveSubstrings mark a name as sensitive wherever they appear.
var sensitiveSubstrings = []string{
	"SECRET", "TOKEN", "PASSWORD", "PASSWD", "PASSPHRASE", "PRIVATE", "CREDENTIAL",
	"APIKEY", "API_KEY", "ACCESSKEY", "ACCESS_KEY", "CONNECTION_STRING", "CONNSTR",
	"SIGNING", "SIGNATURE", "WEBHOOK", "COOKIE", "SESSION", "CERT",
}

// sensitiveTokens mark a name as sensitive when they are a whole word
// (separated by '_', '-', '.' or at the ends), e.g. DB_PWD or AUTH_HEADER.
var sensitiveTokens = map[string]bool{
	"KEY": true, "PWD": true, "PASS": true, "AUTH": true, "DSN": true, "SALT": true,
	"JWT": true, "OTP": true, "PIN": true, "HMAC": true, "BEARER": true, "LICENSE": true,
}

// knownSecretPrefixes are prefixes of well-known credential formats. A value
// starting with one is never shown, whatever its name.
var knownSecretPrefixes = []string{
	"sk_live_", "sk_test_", "rk_live_", "pk_live_", "whsec_", "ghp_", "gho_", "ghu_", "ghs_", "ghr_",
	"github_pat_", "glpat-", "xoxb-", "xoxp-", "xoxa-", "xapp-", "AKIA", "ASIA", "AIza", "ya29.",
	"npm_", "pypi-", "SG.", "-----BEGIN",
}

// IsSensitiveName reports whether a variable name suggests a secret value.
// It errs on the side of "sensitive": masking a harmless value costs little,
// showing a secret is unacceptable.
func IsSensitiveName(name string) bool {
	upper := strings.ToUpper(name)
	for _, s := range sensitiveSubstrings {
		if strings.Contains(upper, s) {
			return true
		}
	}
	for _, tok := range strings.FieldsFunc(upper, func(r rune) bool { return r == '_' || r == '-' || r == '.' }) {
		if sensitiveTokens[tok] {
			return true
		}
	}
	return false
}

// Display is the only function allowed to turn a variable's value into output
// text. It returns:
//   - "(empty)" for an empty value;
//   - for a URL with a scheme and host: scheme://host/path, never userinfo,
//     query or fragment, with secret-looking path segments replaced by "…"
//     (if the name is not sensitive);
//   - the value itself, if the name is not sensitive, the value has at most
//     MaxPlainLength characters, a single line, no control characters, no
//     userinfo, no known credential prefix and does not look random
//     (16+ characters mixing upper case, lower case and digits);
//   - otherwise a placeholder such as "•••••• (32 chars)".
//
// Any value LooksLikeSecret flags is always masked, whatever its name.
func Display(name, value string) string {
	if value == "" {
		return "(empty)"
	}
	if IsSensitiveName(name) || hasSecretPrefix(value) || !printable(value) {
		return Placeholder(value)
	}
	if _, secret := LooksLikeSecret(name, value); secret {
		return Placeholder(value)
	}
	if u, ok := parseURL(value); ok {
		return safeURL(u)
	}
	if hasUserinfo(value) || utf8.RuneCountInString(value) > MaxPlainLength || randomLooking(value) {
		return Placeholder(value)
	}
	return value
}

// randomLooking flags token-like values (16+ characters mixing upper case,
// lower case and digits, without spaces) even under harmless names. This is
// stricter than the length limit alone, in the safe direction.
func randomLooking(v string) bool {
	if utf8.RuneCountInString(v) < 16 || strings.ContainsAny(v, " \t") {
		return false
	}
	var upper, lower, digit bool
	for _, r := range v {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	return upper && lower && digit
}

// Placeholder returns a fixed redaction marker with the value's length.
func Placeholder(value string) string {
	n := utf8.RuneCountInString(value)
	unit := "chars"
	if n == 1 {
		unit = "char"
	}
	return fmt.Sprintf("•••••• (%d %s)", n, unit)
}

func hasSecretPrefix(v string) bool {
	for _, p := range knownSecretPrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// printable rejects multi-line values and control characters (including
// ANSI escape sequences, which could rewrite the terminal).
func printable(v string) bool {
	if !utf8.ValidString(v) {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || unicode.Is(unicode.Bidi_Control, r) {
			return false
		}
	}
	return true
}

func parseURL(v string) (*url.URL, bool) {
	if !strings.Contains(v, "://") {
		return nil, false
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, false
	}
	return u, true
}

// hasUserinfo catches user:pass@host shapes that url.Parse rejects.
func hasUserinfo(v string) bool {
	at := strings.LastIndex(v, "@")
	if at <= 0 {
		return false
	}
	return strings.Contains(v[:at], ":")
}

func safeURL(u *url.URL) string {
	var b strings.Builder
	b.WriteString(u.Scheme)
	b.WriteString("://")
	b.WriteString(u.Host)
	if u.Path != "" && u.Path != "/" {
		for _, seg := range strings.Split(strings.TrimPrefix(u.Path, "/"), "/") {
			b.WriteByte('/')
			if looksSecret(seg) {
				b.WriteString("…")
			} else {
				b.WriteString(seg)
			}
		}
	} else if u.Path == "/" {
		b.WriteByte('/')
	}
	return b.String()
}

// looksSecret flags path segments that may be tokens (e.g. webhook URLs):
// long segments, or medium ones mixing letters and digits.
func looksSecret(seg string) bool {
	n := utf8.RuneCountInString(seg)
	if n > MaxPlainLength {
		return true
	}
	if n < 12 {
		return false
	}
	var letters, digits bool
	for _, r := range seg {
		switch {
		case unicode.IsLetter(r):
			letters = true
		case unicode.IsDigit(r):
			digits = true
		}
	}
	return letters && digits
}
