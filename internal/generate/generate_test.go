package generate

import "testing"

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
