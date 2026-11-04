//go:build unix

package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

// TestFIFONeverOpened: opening a FIFO would block forever, so non-regular
// files must be skipped before any read.
func TestFIFONeverOpened(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, ".env"), 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.js"), 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	write(t, root, "ok.js", "x")
	inv := discover(t, Options{Root: root})
	if got := paths(inv.Files); !slices.Equal(got, []string{"ok.js"}) {
		t.Errorf("files = %v", got)
	}
	for _, p := range []string{".env", "pipe.js"} {
		if r := skipReason(inv, p); r != SkipNotRegular {
			t.Errorf("%s skip = %q, want not-regular-file", p, r)
		}
	}
}

func TestUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read everything")
	}
	root := t.TempDir()
	write(t, root, "locked/a.js", "x")
	write(t, root, "ok.js", "x")
	locked := filepath.Join(root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	inv := discover(t, Options{Root: root})
	if got := paths(inv.Files); !slices.Equal(got, []string{"ok.js"}) {
		t.Errorf("files = %v", got)
	}
	if r := skipReason(inv, "locked"); r != SkipUnreadable {
		t.Errorf("locked skip = %q, want unreadable", r)
	}
}
