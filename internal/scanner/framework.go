package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// framework is the client-side framework of an app root, detected from the
// dependencies in its package.json.
type framework string

const (
	fwNone framework = ""
	fwNext framework = "next"
	fwVite framework = "vite"
)

// detectFrameworks reads package.json once per app root that has JS-family
// sources. Next.js wins over Vite when both are listed.
func detectFrameworks(root string, files []discovery.File) map[string]framework {
	out := map[string]framework{}
	for _, f := range files {
		if _, done := out[f.AppRoot]; done || !isJSFamily(f.Language) {
			continue
		}
		out[f.AppRoot] = readFramework(filepath.Join(root, filepath.FromSlash(f.AppRoot), "package.json"))
	}
	return out
}

func readFramework(p string) framework {
	data, err := os.ReadFile(p) // #nosec G304 -- package.json inside the scanned root
	if err != nil || len(data) > 1<<20 {
		return fwNone
	}
	var pkg struct {
		Dependencies    map[string]any `json:"dependencies"`
		DevDependencies map[string]any `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return fwNone
	}
	has := func(dep string) bool {
		_, a := pkg.Dependencies[dep]
		_, b := pkg.DevDependencies[dep]
		return a || b
	}
	switch {
	case has("next"):
		return fwNext
	case has("vite"):
		return fwVite
	default:
		return fwNone
	}
}

func isJSFamily(l discovery.Language) bool {
	switch l {
	case discovery.LangJavaScript, discovery.LangTypeScript, discovery.LangVue, discovery.LangSvelte:
		return true
	}
	return false
}

var useClientRE = regexp.MustCompile(`(?m)^\s*["']use client["']`)

// note explains client-side reads that the framework will not expose:
//
//   - Next.js inlines only NEXT_PUBLIC_* (and NODE_ENV) into browser code.
//     A file is client code if it has a "use client" directive, or lives under
//     a pages/ directory but not pages/api/.
//   - Vite exposes only VITE_* (plus MODE, BASE_URL, PROD, DEV, SSR) via
//     import.meta.env. Files under the app's src/ are treated as client code.
//
// This is a heuristic for doctor; it never produces findings on its own.
func (fw framework) note(f discovery.File, src []byte, u *Usage) string {
	if fw == fwNone || u.Dynamic || !isJSFamily(f.Language) {
		return ""
	}
	rel := strings.TrimPrefix(f.Path, f.AppRoot+"/")
	if f.AppRoot == "." {
		rel = f.Path
	}
	switch fw {
	case fwNext:
		if strings.HasPrefix(u.Variable, "NEXT_PUBLIC_") || u.Variable == "NODE_ENV" {
			return ""
		}
		client := useClientRE.Match(src[:min(len(src), 512)]) ||
			(hasDir(rel, "pages") && !hasDir(rel, "pages/api"))
		if client {
			return fmt.Sprintf("Next.js only inlines NEXT_PUBLIC_* variables into client code; %s will be undefined in the browser", u.Variable)
		}
	case fwVite:
		if strings.HasPrefix(u.Variable, "VITE_") {
			return ""
		}
		if strings.HasPrefix(rel, "src/") {
			return fmt.Sprintf("Vite only exposes VITE_* variables to client code; %s will be undefined in the browser", u.Variable)
		}
	}
	return ""
}

// hasDir reports whether rel lies under a directory path dir (at any depth,
// e.g. "src/pages/x.tsx" is under "pages").
func hasDir(rel, dir string) bool {
	d := path.Dir(rel)
	return d == dir || strings.HasPrefix(d, dir+"/") || strings.Contains("/"+d+"/", "/"+dir+"/")
}
