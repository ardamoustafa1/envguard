package mask

import (
	"strings"
	"testing"
)

func TestIsSensitiveName(t *testing.T) {
	sensitive := []string{
		"API_KEY", "STRIPE_SECRET_KEY", "GITHUB_TOKEN", "DB_PASSWORD", "MYSQL_PASSWD", "DB_PWD",
		"PRIVATE_KEY_PEM", "AWS_ACCESS_KEY_ID", "GOOGLE_APPLICATION_CREDENTIALS", "AUTH_HEADER",
		"SENTRY_DSN", "CONNECTION_STRING", "SLACK_WEBHOOK_URL", "SESSION_COOKIE", "jwt_secret",
		"apiKey", "KEY", "SMTP_PASS", "TLS_CERT", "HMAC_KEY",
	}
	plain := []string{"PORT", "NODE_ENV", "DATABASE_URL", "LOG_LEVEL", "DEBUG", "HOST", "APP_NAME", "MONKEY_COUNT", "PASSAGE", "AUTHOR"}
	for _, n := range sensitive {
		if !IsSensitiveName(n) {
			t.Errorf("IsSensitiveName(%q) = false, want true", n)
		}
	}
	for _, n := range plain {
		if IsSensitiveName(n) {
			t.Errorf("IsSensitiveName(%q) = true, want false", n)
		}
	}
}

func TestDisplay(t *testing.T) {
	// Secret-looking values are built at runtime (CLAUDE.md conventions).
	stripe := "sk_" + "live_" + strings.Repeat("a1", 12)
	aws := "AKIA" + "IOSFODNN7" + "EXAMPLE"
	pwURL := "postgres://admin:" + "hunter2" + "@db.internal:5432/app?sslmode=require&password=x"

	tests := []struct {
		name, key, value, want string
	}{
		{"empty", "PORT", "", "(empty)"},
		{"short plain", "PORT", "8080", "8080"},
		{"exactly max length", "APP", strings.Repeat("a", 24), strings.Repeat("a", 24)},
		{"too long", "APP", strings.Repeat("a", 25), "•••••• (25 chars)"},
		{"sensitive short", "DB_PASSWORD", "x", "•••••• (1 char)"},
		{"sensitive long", "API_KEY", strings.Repeat("k", 32), "•••••• (32 chars)"},
		{"unicode length", "API_KEY", "日本語", "•••••• (3 chars)"},
		{"known prefix under neutral name", "VALUE", stripe, "•••••• (32 chars)"},
		{"aws key under neutral name", "ID", aws, "•••••• (20 chars)"},
		{"url with password is fully masked", "DATABASE_URL", pwURL, "•••••• (72 chars)"},
		{"url strips query and fragment", "SITE", "https://example.com/app?ref=abc#top", "https://example.com/app"},
		{"url root", "SITE", "https://example.com/", "https://example.com/"},
		{"url no path", "SITE", "https://example.com", "https://example.com"},
		{"url long ok path", "SITE", "https://example.com/api/v1/users", "https://example.com/api/v1/users"},
		{"url secret path segments", "HOOK", "https://hooks.example.com/services/T0123ABCD/B0123ABCD/" + strings.Repeat("x9", 12), "https://hooks.example.com/services/T0123ABCD/B0123ABCD/…"},
		{"url under sensitive name", "SENTRY_DSN", "https://abc@o1.ingest.example.io/42", "•••••• (35 chars)"},
		{"userinfo without scheme", "DB", "user:" + "pw" + "@host", "•••••• (12 chars)"},
		{"multiline", "CERT_BODY", "a\nb", "•••••• (3 chars)"},
		{"control chars", "NAME", "a\x1b[31mb", "•••••• (7 chars)"},
		{"newline neutral", "NAME", "a\nb", "•••••• (3 chars)"},
		{"bidi control", "NAME", "ab\u202ecd", "•••••• (5 chars)"},
		{"invalid utf8", "NAME", "a\xffb", "•••••• (3 chars)"},
		{"random looking short", "NAME", "Ab3" + "dE5fG7hJ9kL1mN2", "•••••• (18 chars)"},
		{"version string shown", "VERSION", "v1.2.3-beta.4+build.20", "v1.2.3-beta.4+build.20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Display(tt.key, tt.value); got != tt.want {
				t.Errorf("Display(%q, ...) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// TestDisplayNeverLeaksSecrets checks that sensitive values never appear in
// Display output, whatever their shape.
func TestDisplayNeverLeaksSecrets(t *testing.T) {
	secret := "zq" + "9" + "SECRETVALUE" + "x7"
	values := []string{
		secret,
		"https://user:" + secret + "@example.com/",
		"https://example.com/?token=" + secret,
		"https://example.com/#" + secret,
		"https://example.com/" + secret + secret,
		"postgres://" + secret + "@db/x",
	}
	for _, v := range values {
		for _, key := range []string{"API_TOKEN", "DATABASE_URL", "X"} {
			got := Display(key, v)
			if strings.Contains(got, secret) {
				t.Errorf("Display(%q, %q) leaked: %q", key, v, got)
			}
		}
	}
}
