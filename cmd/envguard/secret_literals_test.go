package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// fullTokenPatterns match complete credentials of the kind GitHub push
// protection and other secret scanners block. They are assembled from parts
// so that this file does not contain a match itself.
var fullTokenPatterns = map[string]*regexp.Regexp{
	"Stripe key":        regexp.MustCompile(`\b[rs]k_` + `live_[A-Za-z0-9]{20,}`),
	"AWS access key ID": regexp.MustCompile(`\b(?:AKIA|ASIA)` + `[0-9A-Z]{16}\b`),
	"GitHub token":      regexp.MustCompile(`\bgh[pousr]_` + `[A-Za-z0-9]{36,}`),
	"GitHub PAT":        regexp.MustCompile(`\bgithub_pat_` + `[A-Za-z0-9_]{50,}`),
	"Slack token":       regexp.MustCompile(`\bxox[abprs]-` + `[A-Za-z0-9-]{20,}`),
	"Google API key":    regexp.MustCompile(`\bAIza` + `[0-9A-Za-z_-]{35}`),
	"SendGrid key":      regexp.MustCompile(`\bSG\.` + `[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{30,}`),
	"Anthropic key":     regexp.MustCompile(`\bsk-` + `ant-[a-z0-9]+-[A-Za-z0-9_-]{30,}`),
	"OpenAI key":        regexp.MustCompile(`\bsk-` + `(?:proj-)?[A-Za-z0-9_-]{40,}`),
	"private key body":  regexp.MustCompile(`-{5}BEGIN [A-Z ]*PRIVATE KEY-{5}\s+[A-Za-z0-9+/=]{40,}`),
	"JWT":               regexp.MustCompile(`\bey` + `J[A-Za-z0-9_-]{15,}\.ey` + `J[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{10,}`),
}

// TestNoSecretShapedLiteralsInRepository: no committed file may contain a
// complete credential-shaped string. Fixtures build them at run time
// ({{secret:KIND}}, {{placeholder:KIND}}, testutil.FakeSecret), because a
// push containing one is blocked by GitHub secret scanning, even when the
// value is an obvious placeholder.
func TestNoSecretShapedLiteralsInRepository(t *testing.T) {
	checked := 0
	err := filepath.WalkDir(repoRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
			return nil
		}
		data, err := os.ReadFile(p) //nolint:gosec // test
		if err != nil {
			return err
		}
		if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
			return nil // binary
		}
		checked++
		for name, re := range fullTokenPatterns {
			if loc := re.FindIndex(data); loc != nil {
				line := 1 + bytes.Count(data[:loc[0]], []byte("\n"))
				rel, _ := filepath.Rel(repoRoot, p)
				t.Errorf("%s:%d contains a %s-shaped literal; build it at run time instead", filepath.ToSlash(rel), line, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 500 {
		t.Fatalf("only %d files checked", checked)
	}
}
