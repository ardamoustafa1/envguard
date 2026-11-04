package testutil

import (
	"strings"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/mask"
)

// TestFakeSecretsAreDetected keeps FakeSecret honest: every kind must look
// like a secret to mask, or leak tests would test nothing.
func TestFakeSecretsAreDetected(t *testing.T) {
	for i, kind := range SecretKinds {
		if _, ok := mask.LooksLikeSecret("API_SECRET", FakeSecret(kind, i, "")); !ok {
			t.Errorf("FakeSecret(%s) is not detected", kind)
		}
	}
	for i := range 3 * len(canaryKinds) {
		v := Canary(i)
		if !strings.Contains(v, "CNRY") {
			t.Errorf("canary %d has no marker: kind %s", i, CanaryKind(i))
		}
		if _, ok := mask.LooksLikeSecret("NEUTRAL_NAME", v); !ok {
			if _, ok := mask.LooksLikeSecret("API_SECRET", v); !ok {
				t.Errorf("canary of kind %s is not detected as a secret", CanaryKind(i))
			}
		}
		if got := mask.Display("NEUTRAL_NAME", v); strings.Contains(got, "CNRY") {
			t.Errorf("Display leaks canary of kind %s: %q", CanaryKind(i), got)
		}
	}
}
