package mask

import (
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// secretPattern is a known credential format.
type secretPattern struct {
	kind string // human description: "looks like a <kind>"
	re   *regexp.Regexp
}

// secretPatterns are checked in order; more specific formats come first.
// Patterns match anywhere in the value so that e.g. a token inside a URL or
// a quoted JSON blob is still found.
var secretPatterns = []secretPattern{
	{"private key (PEM block)", regexp.MustCompile(`-{5}BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY(?: BLOCK)?-{5}`)},
	{"AWS access key ID", regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16}\b`)},
	{"GitHub personal access token", regexp.MustCompile(`\b(?:ghp_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})\b`)},
	{"GitHub OAuth or app token", regexp.MustCompile(`\bgh[ousr]_[A-Za-z0-9]{36,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`)},
	{"Slack webhook URL", regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Za-z0-9]+/B[A-Za-z0-9]+/[A-Za-z0-9]{16,}`)},
	{"Stripe live secret key", regexp.MustCompile(`\b(?:sk|rk)_live_[A-Za-z0-9]{16,}\b`)},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-[a-z]+\d*-[A-Za-z0-9_-]{20,}`)},
	{"OpenAI-style API key", regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_-]{20,}`)},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"SendGrid API key", regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}\b`)},
	{"Twilio API key", regexp.MustCompile(`\bSK[0-9a-fA-F]{32}\b`)},
	{"JSON Web Token", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)},
}

// MinEntropyLength and MinEntropy configure the generic high-entropy check,
// which only applies under sensitive names (see IsSensitiveName).
const (
	MinEntropyLength = 20
	MinEntropy       = 3.5
)

// LooksLikeSecret reports whether value looks like a real credential and, if
// so, what kind ("GitHub personal access token"). It checks known provider
// formats, PEM private keys, JWTs, URLs with embedded passwords and, for
// sensitive names only, values of at least MinEntropyLength characters with
// Shannon entropy >= MinEntropy. Placeholders (see IsPlaceholder) never
// count. The returned kind never contains any part of the value.
func LooksLikeSecret(key, value string) (kind string, ok bool) {
	v := strings.TrimSpace(value)
	if IsPlaceholder(key, v) {
		return "", false
	}
	for _, p := range secretPatterns {
		if m := p.re.FindString(v); m != "" && !placeholderBody(m) {
			return p.kind, true
		}
	}
	if urlHasRealPassword(v) {
		return "URL with an embedded password", true
	}
	if IsSensitiveName(key) && utf8.RuneCountInString(v) >= MinEntropyLength && !strings.ContainsAny(v, " \t") &&
		Entropy(v) >= MinEntropy {
		return "high-entropy secret", true
	}
	return "", false
}

// Entropy returns the Shannon entropy of s in bits per character.
func Entropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := map[rune]int{}
	n := 0
	for _, r := range s {
		counts[r]++
		n++
	}
	var h float64
	for _, c := range counts {
		p := float64(c) / float64(n)
		h -= p * math.Log2(p)
	}
	return h
}

// urlHasRealPassword reports a URL whose password is not a placeholder, not
// equal to the user name and not on a local host (local development
// credentials such as postgres:postgres@localhost are not secrets).
func urlHasRealPassword(v string) bool {
	if !strings.Contains(v, "://") {
		return false
	}
	u, err := url.Parse(v)
	if err != nil || u.User == nil || isLocalHost(u.Hostname()) {
		return false
	}
	pw, ok := u.User.Password()
	return ok && pw != "" && pw != u.User.Username() && !IsPlaceholder("", pw)
}

var (
	placeholderExact = map[string]bool{
		"changeme": true, "change-me": true, "change_me": true, "replace-me": true, "replace_me": true,
		"replaceme": true, "todo": true, "tbd": true, "test": true, "dummy": true, "example": true,
		"placeholder": true, "secret": true, "password": true, "none": true, "null": true, "nil": true,
		"undefined": true, "false": true, "true": true, "...": true, "-": true,
	}
	placeholderWords = []string{"example", "dummy", "placeholder", "changeme", "change-me", "change_me",
		"replace-me", "replace_me", "replaceme", "your-", "your_", "insert-", "insert_"}
	placeholderTokens = map[string]bool{"test": true, "todo": true, "fake": true, "sample": true, "xxx": true}
	angleRE           = regexp.MustCompile(`^<[^<>]+>$|^\[[^\[\]]+\]$|^\{\{[^{}]+\}\}$|^\$\{[^{}]+\}$`)
	repeatedRE        = regexp.MustCompile(`(?i)x{4,}|\*{3,}|0{8,}|\.{3,}`)
	tokenSplitRE      = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// IsPlaceholder reports whether a value is obviously not a real secret, so
// templates such as `API_KEY=your-api-key` stay quiet: empty values,
// changeme / replace-me / todo / test / dummy / example / placeholder,
// your-..., <...>, [..], {{...}}, ${...}, xxxx..., ***, values equal to the
// key name, and localhost URLs without a password.
func IsPlaceholder(key, value string) bool {
	v := strings.TrimSpace(value)
	lower := strings.ToLower(v)
	switch {
	case v == "":
		return true
	case placeholderExact[lower]:
		return true
	case key != "" && strings.EqualFold(v, key):
		return true
	case angleRE.MatchString(v):
		return true
	case repeatedRE.MatchString(v):
		return true
	}
	for _, w := range placeholderWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	for _, tok := range tokenSplitRE.Split(lower, -1) {
		if placeholderTokens[tok] {
			return true
		}
	}
	if strings.Contains(v, "://") {
		if u, err := url.Parse(v); err == nil && isLocalHost(u.Hostname()) && u.User == nil {
			return true
		}
	}
	return false
}

// placeholderBody catches provider-shaped placeholders such as
// sk_live_xxxxxxxxxxxxxxxx or ghp_000000000000....
func placeholderBody(match string) bool {
	return repeatedRE.MatchString(match) || strings.Contains(strings.ToLower(match), "example")
}

func isLocalHost(h string) bool {
	switch strings.ToLower(h) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "host.docker.internal":
		return true
	}
	return strings.HasSuffix(strings.ToLower(h), ".localhost")
}
