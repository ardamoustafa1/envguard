package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestActionReleaseDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock executable uses a POSIX script")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct{ Name, Run string }
		}
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "action-test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{}
	for name, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.Name == "Is there a release yet?" {
				scripts[name] = step.Run
			}
		}
	}
	if len(scripts) != 2 {
		t.Fatalf("release availability guards = %d, want 2", len(scripts))
	}
	var action struct {
		Runs struct{ Steps []struct{ Name, Run string } }
	}
	data, err = os.ReadFile(filepath.Join(repoRoot, "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &action); err != nil {
		t.Fatal(err)
	}
	for _, step := range action.Runs.Steps {
		if step.Name == "Resolve version and platform" {
			scripts["resolver"] = step.Run
		}
	}
	if scripts["resolver"] == "" {
		t.Fatal("composite version resolver missing")
	}
	for name, script := range scripts {
		for _, tt := range []struct {
			name, url, curlCode string
			exists              bool
		}{
			{"empty releases redirects with HTTP 200", "https://github.com/ardamoustafa1/envguard/releases", "0", false},
			{"published release tag", "https://github.com/ardamoustafa1/envguard/releases/tag/v1.2.3", "0", true},
			{"HTTP error", "", "22", false},
			{"unexpected redirect", "https://example.invalid/releases/tag/v1.2.3", "0", false},
		} {
			t.Run(name+"/"+tt.name, func(t *testing.T) {
				dir := t.TempDir()
				curl := filepath.Join(dir, "curl")
				if err := os.WriteFile(curl, []byte("#!/bin/sh\nprintf '%s' \"$MOCK_RELEASE_URL\"\nexit \"$MOCK_CURL_EXIT\"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				output := filepath.Join(dir, "github-output")
				cmd := exec.Command(bash, "-c", script)
				cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
					"MOCK_RELEASE_URL="+tt.url, "MOCK_CURL_EXIT="+tt.curlCode,
					"GITHUB_OUTPUT="+output, "GITHUB_REPOSITORY=ardamoustafa1/envguard",
					"INPUT_VERSION=latest", "RUNNER_OS=Linux", "RUNNER_ARCH=X64", "TOOL_CACHE="+dir)
				out, err := cmd.CombinedOutput()
				result, readErr := os.ReadFile(output)
				if name != "resolver" {
					want := "exists=false"
					if tt.exists {
						want = "exists=true"
					}
					if err != nil || readErr != nil || strings.TrimSpace(string(result)) != want {
						t.Errorf("release guard: exit %v, output %q, log %s; want %s", err, result, out, want)
					}
					return
				}
				if tt.exists {
					if err != nil || !strings.Contains(string(result), "version=1.2.3\n") {
						t.Errorf("version resolver: exit %v, output %q, log %s", err, result, out)
					}
				} else if err == nil || !strings.Contains(string(out), "no published release") {
					t.Errorf("version resolver must explain missing release: exit %v, log %s", err, out)
				}
			})
		}
	}
}

func TestReleaseUsesLatestPatchedGo(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string
				With map[string]any
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "actions/setup-go@") {
				continue
			}
			count++
			version, _ := step.With["go-version"].(string)
			if version != "stable" && version != "oldstable" {
				t.Errorf("release compiler must follow stable/oldstable security patches, got %q (version file: %v)", version, step.With["go-version-file"])
			}
		}
	}
	if count == 0 {
		t.Fatal("release has no explicit Go setup step")
	}
}

func TestHomebrewPostflightLoadsInInstallStepsDSL(t *testing.T) {
	ruby, err := exec.LookPath("ruby")
	if err != nil {
		t.Skip("ruby not installed")
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var release struct {
		Casks []struct {
			Hooks struct {
				Post struct {
					InstallSteps string `yaml:"install_steps"`
				}
			}
		} `yaml:"homebrew_casks"`
	}
	if err := yaml.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	if len(release.Casks) == 0 || release.Casks[0].Hooks.Post.InstallSteps == "" {
		t.Fatal("Homebrew postflight hook missing")
	}
	// Homebrew InstallSteps evaluates this block before a staged_path context
	// exists. Exercise its Ruby semantics without touching any installed file.
	const context = `
module OS
  def self.mac?; true; end
end
HOMEBREW_PREFIX = "/isolated"
class InstallStepsContext
  def run(path, args:)
    ::STDOUT.write(([path] + args).join("\0"))
  end
end
InstallStepsContext.new.instance_eval(ARGV.fetch(0), "postflight_steps", 1)
`
	cmd := exec.Command(ruby, "-e", context, release.Casks[0].Hooks.Post.InstallSteps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("postflight hook cannot load: %v: %s", err, out)
	}
	want := "/usr/bin/xattr\x00-dr\x00com.apple.quarantine\x00/isolated/bin/envguard"
	if string(out) != want {
		t.Errorf("postflight command = %q, want %q", out, want)
	}
}

func TestActionThresholdWithLeadingFlags(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Runs struct {
			Steps []struct{ Name, Run string }
		}
	}
	if err := yaml.Unmarshal(data, &action); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range action.Runs.Steps {
		if step.Name == "Run EnvGuard" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("Run EnvGuard step not found")
	}
	for _, tt := range []struct {
		args      string
		threshold string
		append    bool
	}{
		{"doctor", "error", true},
		{"--no-color doctor", "error", true},
		{"--ascii --verbose check", "warning", true},
		{"--root doctor diff", "error", false},
		{"--config doctor version", "error", false},
		{"--no-color doctor --fail-on never", "error", false},
		{"--no-color doctor --fail-on=never", "error", false},
		{"--no-color --format json", "error", true},
		{"--no-color doctor", "", false},
		{"", "error", true},
	} {
		t.Run(tt.args+"/"+tt.threshold, func(t *testing.T) {
			// Capture the argv delivered by the exact composite action shell.
			cmd := exec.Command(bash, "-c", "envguard() { printf '%s\\0' \"$@\"; };\n"+script)
			cmd.Env = append(os.Environ(), "WD=fixture", "ARGS="+tt.args, "FAIL_ON="+tt.threshold)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("action shell: %v: %s", err, out)
			}
			args := strings.Fields(tt.args)
			if len(args) == 0 {
				args = []string{"check"}
			}
			want := append([]string{"--root", "fixture"}, args...)
			if tt.append {
				want = append(want, "--fail-on", tt.threshold)
			}
			got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
			if !slices.Equal(got, want) {
				t.Errorf("argv = %q, want %q", got, want)
			}
		})
	}
}

func TestNPMLauncherUnsupportedPlatformHelp(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	template, err := os.ReadFile(filepath.Join(repoRoot, "npm", "envguard", "bin", "envguard.js"))
	if err != nil {
		t.Fatal(err)
	}
	launcher := strings.NewReplacer("__PLATFORM_PACKAGES__", `{"darwin-arm64":"envguard-cli-darwin-arm64"}`,
		"__MAIN_PACKAGE__", `"envguard-cli"`).Replace(string(template))
	file := filepath.Join(t.TempDir(), "envguard.cjs")
	if err := os.WriteFile(file, []byte(launcher), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "-e", `Object.defineProperty(process, "platform", {value:"freebsd"}); delete process.env.ENVGUARD_BINARY; require(process.argv[1]);`, file)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != exitError {
		t.Fatalf("unsupported platform exit = %v: %s", err, out)
	}
	if !strings.Contains(string(out), "https://github.com/ardamoustafa1/envguard/blob/main/docs/installation.md") ||
		strings.Contains(string(out), "YOUR_GITHUB_USER") {
		t.Errorf("unsupported platform help has no usable installation link: %s", out)
	}
}
