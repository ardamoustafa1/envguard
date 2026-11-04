package main

import (
	"bytes"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/ardamoustafa1/envguard"

// netAllowlist lists third-party packages permitted to import "net" itself.
// Each entry must be justified: it may only use pure helpers (parsing),
// never open connections.
var netAllowlist = map[string]string{
	// cobra's flag library; uses net.ParseIP / net.IPMask for IP flag types.
	"github.com/spf13/pflag": "IP flag parsing only",
	// Windows-only: the syscall bindings pulled in by termenv and x/term for
	// console modes. They define socket-related types in the same package;
	// EnvGuard calls only the console functions, never a socket one.
	"golang.org/x/sys/windows": "Windows syscall bindings; only console functions are used",
}

// networkClients may not appear anywhere in the dependency graph.
func isNetworkClient(pkg string) bool {
	for _, p := range []string{"net/http", "net/rpc", "net/smtp", "net/mail", "crypto/tls"} {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
}

// TestOffline enforces principle 1 (offline only) on every non-test package
// of the module and its transitive dependencies.
func TestOffline(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not available")
	}
	cmd := exec.Command(goBin, "list", "-deps", "-f", "{{.ImportPath}}\t{{join .Imports \",\"}}", modulePath+"/...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}

	ours := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		pkg, importList, _ := strings.Cut(line, "\t")
		imports := strings.Split(importList, ",")
		isOurs := pkg == modulePath || strings.HasPrefix(pkg, modulePath+"/")
		if isOurs {
			ours++
		}

		if isNetworkClient(pkg) {
			t.Errorf("network client package %s is in the dependency graph", pkg)
		}
		for _, imp := range imports {
			if isNetworkClient(imp) {
				t.Errorf("%s imports network client %s", pkg, imp)
			}
		}
		if slices.Contains(imports, "net") {
			switch {
			case isOurs:
				t.Errorf("%s imports net: EnvGuard must not import net (principle 1)", pkg)
			case netAllowlist[pkg] == "" && !strings.HasPrefix(pkg, "vendor/") && !isStdlib(pkg):
				t.Errorf("third-party %s imports net; review it and add it to netAllowlist with a justification, or drop the dependency", pkg)
			}
		}
	}
	if ours == 0 {
		t.Fatal("go list returned no packages of this module")
	}
}

// isStdlib is a cheap check: standard library import paths have no dot in
// their first element.
func isStdlib(pkg string) bool {
	first, _, _ := strings.Cut(pkg, "/")
	return !strings.Contains(first, ".")
}
