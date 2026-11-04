package scanner

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// library is a bit set of libraries an app depends on, read from its
// dependency files. Some detectors are ambiguous on their own (django-environ's
// bare env("X") looks like any function call); knowing the app depends on the
// library lets them run in files that do not import it themselves.
type library uint8

const (
	// libDjangoEnviron: django-environ. Settings are usually split over
	// several modules (base.py, production.py, ...) of which only one does
	// `import environ`; the others import the configured `env` object.
	libDjangoEnviron library = 1 << iota
)

// pythonDependencyFiles are searched, relative to an app root.
var pythonDependencyGlobs = []string{
	"requirements*.txt", "requirements/*.txt", "pyproject.toml", "Pipfile", "setup.py", "setup.cfg",
}

const maxDependencyFile = 1 << 20

// detectLibraries reads the Python dependency files of every app root that
// has Python sources, once per app root.
func detectLibraries(root string, files []discovery.File) map[string]library {
	out := map[string]library{}
	for _, f := range files {
		if f.Language != discovery.LangPython {
			continue
		}
		if _, done := out[f.AppRoot]; done {
			continue
		}
		out[f.AppRoot] = readLibraries(filepath.Join(root, filepath.FromSlash(f.AppRoot)))
	}
	return out
}

func readLibraries(appDir string) library {
	var libs library
	for _, g := range pythonDependencyGlobs {
		matches, err := filepath.Glob(filepath.Join(appDir, filepath.FromSlash(g)))
		if err != nil {
			continue
		}
		for _, m := range matches {
			info, err := os.Lstat(m)
			if err != nil || !info.Mode().IsRegular() || info.Size() > maxDependencyFile {
				continue // never follow links out of the repository
			}
			data, err := os.ReadFile(m) // #nosec G304 -- dependency file inside the scanned root
			if err != nil {
				continue
			}
			lower := bytes.ToLower(data)
			if bytes.Contains(lower, []byte("django-environ")) || bytes.Contains(lower, []byte("django_environ")) {
				libs |= libDjangoEnviron
			}
		}
	}
	return libs
}
