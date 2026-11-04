// Package config reads wcgrep's configuration from the environment.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the resolved settings.
type Config struct {
	Dir      string
	useColor bool
}

// Load resolves the configuration directory and color setting.
//
// Environment variables (all optional, documented in the README):
//
//	WCGREP_CONFIG_DIR  where to look for the config (default $XDG_CONFIG_HOME/wcgrep)
//	WCGREP_COLOR       always, never or auto (default auto)
//	NO_COLOR           disables color when set to any value
func Load() (*Config, error) {
	dir := os.Getenv("WCGREP_CONFIG_DIR")
	if dir == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("cannot find a home directory: %w", err)
			}
			base = filepath.Join(home, ".config")
		}
		dir = filepath.Join(base, "wcgrep")
	}
	c := &Config{Dir: dir}
	switch strings.ToLower(os.Getenv("WCGREP_COLOR")) {
	case "always":
		c.useColor = true
	case "never":
		c.useColor = false
	default:
		_, noColor := os.LookupEnv("NO_COLOR")
		c.useColor = !noColor && os.Getenv("TERM") != "dumb"
	}
	return c, nil
}

// Color returns an ANSI SGR sequence, or "" when color is off.
func (c *Config) Color(code string) string {
	if !c.useColor {
		return ""
	}
	return "\x1b[" + code + "m"
}
