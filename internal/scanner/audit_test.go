package scanner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

func TestFrameworkMetadataRejectsLinksAndOversizeFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "external-package.json")
	if err := os.WriteFile(target, []byte(`{"dependencies":{"next":"1"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "package.json")
	if err := os.Symlink(target, link); err == nil {
		if got := readFramework(link); got != fwNone {
			t.Errorf("followed external metadata link: %s", got)
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
	data := append([]byte(`{"dependencies":{"next":"1"}}`), make([]byte, maxDependencyFile)...)
	if err := os.WriteFile(link, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readFramework(link); got != fwNone {
		t.Errorf("accepted oversized metadata: %s", got)
	}
}

func TestPythonDependencyDirectoryCannotEscapeRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "extra.txt"), []byte("django-environ==0.12.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	requirements := filepath.Join(root, "requirements")
	if err := os.Symlink(outside, requirements); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if got := readLibraries(root); got != 0 {
		t.Errorf("external requirements directory changed library detection: %v", got)
	}
	if err := os.Remove(requirements); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(requirements, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(requirements, "extra.txt"), []byte("django-environ==0.12.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readLibraries(root); got&libDjangoEnviron == 0 {
		t.Error("normal in-root requirements directory no longer detects django-environ")
	}
}

func TestCustomExtractorsIgnoreNonCode(t *testing.T) {
	tests := []struct {
		name string
		lang discovery.Language
		src  string
		want []string
	}{
		{"destructuring comment comma", discovery.LangJavaScript, `const { REAL /* , FAKE */ } = process.env;`, []string{"REAL"}},
		{"destructuring string comma", discovery.LangJavaScript, `const { REAL = ", FAKE" } = process.env;`, []string{"REAL"}},
		{"destructuring leading comment", discovery.LangJavaScript, "const { /* FAKE */ REAL, // FAKE2\n SECOND } = process.env;", []string{"REAL", "SECOND"}},
		{"destructuring comment equals", discovery.LangJavaScript, `const { REAL /* = default */ } = process.env;`, []string{"REAL"}},
		{"ordinary Go raw string", discovery.LangGo, "package p\nvar example = `env:\"FAKE\"`", nil},
		{"Go tag plus ordinary prefix string", discovery.LangGo, "package p\nvar example = `envPrefix:\"FAKE_\"`\ntype Config struct { Value string `env:\"REAL\"` }", []string{"REAL"}},
		{"Go function literal raw string", discovery.LangGo, "package p\nfunc f() { _ = `env:\"FAKE\"` }\ntype Config struct { Value string `env:\"REAL\"` }", []string{"REAL"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usages := scanFile(tt.lang, "example", []byte(tt.src))
			var names []string
			for _, u := range usages {
				names = append(names, u.Variable)
				if u.Inferred {
					t.Errorf("ordinary string made %s inferred", u.Variable)
				}
				if tt.name == "destructuring comment equals" && u.HasDefault {
					t.Error("a comment made a required read optional")
				}
			}
			if !slices.Equal(names, tt.want) {
				t.Errorf("variables = %v, want %v", names, tt.want)
			}
		})
	}
}
