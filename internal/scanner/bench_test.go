package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// benchSources are realistic ~60-line files per language: real usages,
// defaults, comments and strings with fake usages, and plain code.
var benchSources = map[string]string{
	".ts": `import { db } from "./db";
// process.env.FAKE_IN_COMMENT
const url = process.env.DATABASE_URL_%[1]d;
const port = process.env.PORT || 3000;
const { API_KEY_%[1]d, TIMEOUT = "30" } = process.env;
const msg = "process.env.FAKE_IN_STRING";
const tpl = ` + "`${process.env.PUBLIC_URL}/api`" + `;
export function handler(req: Request): Response {
  const x = req.headers.get("x") ?? "y";
  return new Response(JSON.stringify({ url, port, x, tpl, msg }));
}
`,
	".py": `"""Module docstring mentioning os.getenv('FAKE')."""
import os
# os.environ["FAKE_IN_COMMENT"]
DATABASE_URL = os.environ["DATABASE_URL_%[1]d"]
DEBUG = os.getenv("DEBUG", "false")
def handler(event):
    return {"url": DATABASE_URL, "debug": DEBUG, "text": "os.getenv('FAKE')"}
`,
	".go": `package svc

import "os"

// os.Getenv("FAKE_IN_COMMENT")
type Config struct {
	Port int ` + "`env:\"PORT_%[1]d\" envDefault:\"8080\"`" + `
}

func Load() string {
	return os.Getenv("DATABASE_URL_%[1]d") + "os.Getenv(\"FAKE\")"
}
`,
	".rb": `# ENV["FAKE_IN_COMMENT"]
class Svc%[1]d
  def url = ENV.fetch("DATABASE_URL_%[1]d")
  def port = ENV["PORT"] || 3000
  def text = "ENV['FAKE']"
end
`,
	".sh": `#!/usr/bin/env bash
set -euo pipefail
# $FAKE_IN_COMMENT
TARGET=prod
echo "deploying ${APP_NAME_%[1]d} to $TARGET with ${REGION:-eu}"
echo '$FAKE_IN_SINGLE_QUOTES'
`,
}

func writeBenchTree(tb testing.TB, root string, n int) {
	tb.Helper()
	exts := []string{".ts", ".py", ".go", ".rb", ".sh"}
	for i := range n {
		ext := exts[i%len(exts)]
		dir := filepath.Join(root, fmt.Sprintf("pkg%03d", i/100))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			tb.Fatal(err)
		}
		body := fmt.Sprintf(benchSources[ext], i)
		// Pad to a typical file size with code that matches no detector.
		body += strings.Repeat("function helper(a, b) { return a + b; } // padding line\n", 40)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%05d%s", i, ext)), []byte(body), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
}

const benchFiles = 10_000

func benchInventory(b *testing.B) *discovery.Inventory {
	b.Helper()
	root := b.TempDir()
	writeBenchTree(b, root, benchFiles)
	inv, err := discovery.Discover(discovery.Options{Root: root})
	if err != nil {
		b.Fatal(err)
	}
	if len(inv.Files) != benchFiles {
		b.Fatalf("discovered %d files, want %d", len(inv.Files), benchFiles)
	}
	return inv
}

// BenchmarkScan10kFiles measures Scan alone (file reads included) over
// 10,000 source files.
func BenchmarkScan10kFiles(b *testing.B) {
	inv := benchInventory(b)
	b.ReportAllocs()
	b.ResetTimer()
	var usages int
	for b.Loop() {
		usages = len(Scan(Options{Root: inv.Root, Files: inv.Files}).Usages)
	}
	b.ReportMetric(float64(usages), "usages")
}

// BenchmarkDiscoverAndScan10kFiles measures the full walk plus scan.
func BenchmarkDiscoverAndScan10kFiles(b *testing.B) {
	root := b.TempDir()
	writeBenchTree(b, root, benchFiles)
	b.ResetTimer()
	for b.Loop() {
		inv, err := discovery.Discover(discovery.Options{Root: root})
		if err != nil {
			b.Fatal(err)
		}
		_ = Scan(Options{Root: inv.Root, Files: inv.Files})
	}
}

// TestScanLargeTreeIsFast is a coarse regression guard (the real numbers come
// from the benchmarks): 2,000 files must scan within a generous bound even
// under -race.
func TestScanLargeTreeIsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	root := t.TempDir()
	writeBenchTree(t, root, 2000)
	inv, err := discovery.Discover(discovery.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	res := Scan(Options{Root: inv.Root, Files: inv.Files})
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("scanning 2000 files took %v", d)
	}
	for _, u := range res.Usages {
		if strings.Contains(u.Variable, "FAKE") {
			t.Fatalf("phantom usage %s in %s:%d", u.Variable, u.File, u.Line)
		}
	}
	if len(res.Usages) < 2000 {
		t.Errorf("only %d usages", len(res.Usages))
	}
}
