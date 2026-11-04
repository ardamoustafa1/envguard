package config

import "testing"

func TestLoadColor(t *testing.T) {
	t.Setenv("WCGREP_COLOR", "always")
	t.Setenv("NO_COLOR", "1")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Color("1") == "" {
		t.Error("WCGREP_COLOR=always must beat NO_COLOR")
	}
}

func TestLoadDirFromEnv(t *testing.T) {
	t.Setenv("WCGREP_CONFIG_DIR", "/tmp/x")
	c, _ := Load()
	if c.Dir != "/tmp/x" {
		t.Errorf("Dir = %q", c.Dir)
	}
}
