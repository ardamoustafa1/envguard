package generate

import (
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/config"
)

func TestGeneratedConfigKeepsSourceNamesInComments(t *testing.T) {
	v := &variable{name: "REAL", readNoDflt: true,
		usedAt:     []string{"source\nfail_on: never\n#x.js:1"},
		declaredAt: []string{"template\r\nvariables: bad:1"},
		refAt:      []string{"compose\nversion: 2.yml:1"},
	}
	data := renderConfig(map[string]*variable{"REAL": v}, []string{"REAL"}, &Result{})
	cfg, err := config.Parse(data, ".envguard.yaml")
	if err != nil {
		t.Fatalf("a source filename corrupted generated YAML: %v\n%s", err, data)
	}
	if cfg.FailOn != "" || !cfg.Variables["REAL"].Required {
		t.Errorf("source filename altered settings: %#v", cfg)
	}
	if !strings.Contains(string(data), `source\nfail_on: never\n#x.js:1`) {
		t.Errorf("source filename is not visibly escaped: %s", data)
	}
}

func TestInferType(t *testing.T) {
	tests := []struct {
		name    string
		samples []string
		want    string
	}{
		{"PORT", nil, "port"},
		{"API_PORT", []string{"8080"}, "port"},
		{"DATABASE_URL", nil, "url"},
		{"REDIS_URL", []string{"localhost:6379"}, ""}, // template disagrees with the name
		{"CALLBACK_URI", []string{"https://x.io/cb"}, "url"},
		{"ADMIN_EMAIL", []string{"ops@example.com"}, "email"},
		{"API_HOST", []string{"api.internal"}, "hostname"},
		{"DEBUG", []string{"false"}, "bool"},
		{"FEATURE_ON", []string{"YES"}, "bool"},
		{"WORKERS", []string{"4"}, "int"},
		{"RETRIES", []string{"1"}, "int"}, // 1/0 are ints, not bools
		{"RATIO", []string{"0.5"}, "float"},
		{"HOMEPAGE", []string{"https://example.org"}, "url"},
		{"API_KEY", []string{"your-api-key"}, ""}, // placeholder: no evidence
		{"NAME", []string{"demo"}, ""},
		{"EMPTY", []string{""}, ""},
		{"MIXED", []string{"4", "x"}, ""},
	}
	for _, tt := range tests {
		if got := InferType(tt.name, tt.samples); got != tt.want {
			t.Errorf("InferType(%s, %v) = %q, want %q", tt.name, tt.samples, got, tt.want)
		}
	}
}
