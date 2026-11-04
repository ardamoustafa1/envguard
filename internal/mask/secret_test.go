package mask

import (
	"strings"
	"testing"
)

// Secret-shaped values are assembled at runtime so that no literal in this
// repository trips GitHub push protection or secret scanners.
func rep(s string, n int) string { return strings.Repeat(s, n) }

func TestLooksLikeSecret(t *testing.T) {
	tests := []struct {
		name, key, value, kind string
	}{
		{"aws access key id", "ID", "AKIA" + "Q2ZX7ZX7" + "ZX7R4KPM", "AWS access key ID"},
		{"aws temp key", "ID", "ASIA" + rep("B7", 8), "AWS access key ID"},
		{"github pat classic", "TOKEN_X", "ghp" + "_" + rep("aB3dE5", 6), "GitHub personal access token"},
		{"github fine-grained", "X", "github" + "_pat_" + rep("11AB", 6) + "_" + rep("q9Z", 8), "GitHub personal access token"},
		{"github oauth", "X", "gho" + "_" + rep("Zy8", 12), "GitHub OAuth or app token"},
		{"github server token", "X", "ghs" + "_" + rep("Kp4", 12), "GitHub OAuth or app token"},
		{"slack bot", "X", "xox" + "b-" + "1234567890-" + rep("aBc9", 6), "Slack token"},
		{"slack webhook", "HOOK", "https://hooks.slack.com/services/" + "T01AB2CD3/" + "B04EF5GH6/" + rep("Xy7Qz2", 4), "Slack webhook URL"},
		{"stripe live", "X", "sk" + "_live_" + rep("4eC39H", 5), "Stripe live secret key"},
		{"stripe restricted", "X", "rk" + "_live_" + rep("9Pq2", 6), "Stripe live secret key"},
		{"anthropic", "X", "sk-" + "ant-api03-" + rep("Ab9_", 12), "Anthropic API key"},
		{"openai project", "X", "sk-" + "proj-" + rep("Tq8L", 10), "OpenAI-style API key"},
		{"openai legacy", "X", "sk-" + rep("Wd4r", 12), "OpenAI-style API key"},
		{"google api key", "X", "AI" + "za" + "Sy" + rep("Dq3_k", 6) + "Zx7", "Google API key"},
		{"sendgrid", "X", "S" + "G." + rep("aB3dE5fG7h", 2) + "Jk." + rep("Lm9Np2Qr4s", 4) + "Tu7", "SendGrid API key"},
		{"twilio api key", "X", "S" + "K" + rep("0a1b2c3d", 4), "Twilio API key"},
		{"pem private key", "KEY_PEM", "-----BEGIN " + "RSA PRIVATE" + " KEY-----\n" + rep("MIIE", 10), "private key (PEM block)"},
		{"pem openssh", "K", "-----BEGIN " + "OPENSSH PRIVATE" + " KEY-----", "private key (PEM block)"},
		{"jwt", "SESSION", "ey" + "JhbGciOiJIUzI1NiJ9" + "." + "ey" + "JzdWIiOiIxMjM0NTY3ODkwIn0" + "." + rep("SflKxw", 5), "JSON Web Token"},
		{"url with password", "DATABASE_URL", "postgres://app:" + "Zq8" + "vN2rT5" + "@db.internal:5432/app", "URL with an embedded password"},
		{"high entropy sensitive", "API_SECRET", "q8F2" + "vL9xR4tY7mN1" + "kP5wZ3sB6", "high-entropy secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := LooksLikeSecret(tt.key, tt.value)
			if !ok || kind != tt.kind {
				t.Errorf("LooksLikeSecret = %q, %v; want %q", kind, ok, tt.kind)
			}
			if strings.Contains(kind, tt.value[:min(len(tt.value), 8)]) {
				t.Error("kind leaks the value")
			}
			if got := Display("NEUTRAL", tt.value); strings.Contains(got, tt.value) {
				t.Errorf("Display shows a secret: %q", got)
			}
		})
	}
}

func TestNotSecrets(t *testing.T) {
	tests := []struct{ key, value string }{
		{"API_KEY", ""},
		{"API_KEY", "changeme"},
		{"API_KEY", "change-me"},
		{"API_KEY", "your-api-key"},
		{"API_KEY", "your_api_key_here"},
		{"API_KEY", "<your key>"},
		{"API_KEY", "xxxxxxxxxxxxxxxxxxxxxxxx"},
		{"API_KEY", "***"},
		{"API_KEY", "example"},
		{"API_KEY", "dummy-value-123"},
		{"API_KEY", "test"},
		{"API_KEY", "placeholder"},
		{"API_KEY", "replace-me"},
		{"API_KEY", "TODO"},
		{"API_KEY", "API_KEY"},
		{"API_KEY", "{{ api_key }}"},
		{"STRIPE_KEY", "sk" + "_live_" + rep("x", 24)},
		{"STRIPE_KEY", "sk" + "_test_" + rep("4eC39H", 5)}, // test-mode keys are not live secrets
		{"AWS_ACCESS_KEY_ID", "AKIA" + "IOSFODNN7" + "EXAMPLE"},
		{"DATABASE_URL", "postgres://postgres:postgres@localhost:5432/app"},
		{"DATABASE_URL", "postgres://user:password@db:5432/app"},
		{"DATABASE_URL", "postgres://localhost/app"},
		{"REDIS_URL", "redis://127.0.0.1:6379"},
		{"PORT", "8080"},
		{"LOG_LEVEL", "debug"},
		{"APP_NAME", "my-application-with-a-long-name"},
		{"API_SECRET", "short"},
		{"API_SECRET", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{"DESCRIPTION", "q8F2" + "vL9xR4tY7mN1" + "kP5wZ3sB6"}, // high entropy but neutral name
	}
	for _, tt := range tests {
		if kind, ok := LooksLikeSecret(tt.key, tt.value); ok {
			t.Errorf("LooksLikeSecret(%q, %q) = %q, want no secret", tt.key, tt.value, kind)
		}
	}
}

func TestEntropy(t *testing.T) {
	if e := Entropy(""); e != 0 {
		t.Errorf("empty = %v", e)
	}
	if e := Entropy("aaaa"); e != 0 {
		t.Errorf("aaaa = %v", e)
	}
	if e := Entropy("abcd"); e != 2 {
		t.Errorf("abcd = %v", e)
	}
}
