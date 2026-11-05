package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/generate"
	"github.com/ardamoustafa1/envguard/internal/mask"
	"github.com/ardamoustafa1/envguard/internal/project"
)

type initOptions struct {
	force   bool
	dryRun  bool
	example bool
	app     string
}

func newInitCmd(opts *globalOptions) *cobra.Command {
	iopts := &initOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a starter .envguard.yaml (and optionally .env.example) from what exists",
		Long: "init looks at your templates, the env vars your code reads, Compose files and workflows,\n" +
			"and writes a starter .envguard.yaml: inferred types, required for variables read without a\n" +
			"default, sensitive for secret-looking names, and TODO comments for descriptions.\n\n" +
			"With --example it also writes .env.example (if missing): every variable the code reads,\n" +
			"grouped by prefix, with empty values. Values are never copied from any env file.\n\n" +
			"Existing files are never overwritten without --force. --dry-run prints instead of writing.",
		Example: "  envguard init --dry-run            # preview, writes nothing\n" +
			"  envguard init --example            # also write .env.example if it is missing\n" +
			"  envguard init --app apps/api --force",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd.OutOrStdout(), opts, iopts)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&iopts.force, "force", false, "overwrite existing files")
	f.BoolVar(&iopts.dryRun, "dry-run", false, "print the generated files to stdout instead of writing them")
	f.BoolVar(&iopts.example, "example", false, "also generate .env.example if it does not exist")
	f.StringVar(&iopts.app, "app", "", "generate for one app root in a monorepo (relative to --root)")
	return cmd
}

func runInit(stdout io.Writer, gopts *globalOptions, opts *initOptions) error {
	root := gopts.Root
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return fmt.Errorf("root %q is not a directory", root)
	}
	// init deliberately ignores any existing config (it may be invalid or be
	// the file being replaced) and uses defaults.
	settings := config.Merge(config.Defaults(), nil, config.Flags{})
	settings.Root = abs

	inv, err := discovery.Discover(settings.DiscoveryOptions())
	if err != nil {
		return err
	}
	app, err := resolveApp(opts.app, inv)
	if err != nil {
		return err
	}
	p, err := project.Build(&settings, inv, project.Options{})
	if err != nil {
		return err
	}
	res := generate.Generate(p, generate.Options{Example: opts.example, App: app})

	configPath := filepath.Join(abs, ".envguard.yaml")
	exampleRel := ".env.example"
	if app != "." {
		exampleRel = app + "/.env.example"
	}
	examplePath := filepath.Join(abs, filepath.FromSlash(exampleRel))

	if opts.dryRun {
		_, err := fmt.Fprintf(stdout, "# --- .envguard.yaml ---\n%s", res.Config)
		if err == nil && res.Example != nil {
			_, err = fmt.Fprintf(stdout, "\n# --- %s ---\n%s", mask.Printable(strings.ReplaceAll(exampleRel, "\n", `\n`)), res.Example)
		}
		return err
	}

	if !opts.force {
		for _, name := range []string{".envguard.yaml", ".envguard.yml"} {
			if _, err := os.Stat(filepath.Join(abs, name)); err == nil {
				return fmt.Errorf("%s already exists; use --force to overwrite it, or --dry-run to preview: %w",
					name, errExists)
			}
		}
	}
	if err := writeFile(configPath, res.Config); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "Wrote .envguard.yaml: %s (%d required, %d sensitive).\n",
		plural(res.Variables, "variable", "variables"), res.Required, res.Sensitive)

	if res.Example != nil {
		if _, err := os.Stat(examplePath); err == nil && !opts.force {
			_, _ = fmt.Fprintf(stdout, "Kept the existing %s (use --force to regenerate it).\n", mask.Printable(exampleRel))
		} else {
			if err := writeFile(examplePath, res.Example); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(stdout, "Wrote %s: %s in %s, all with empty values.\n",
				mask.Printable(exampleRel), plural(res.ExampleVariables, "variable", "variables"),
				plural(res.ExampleGroups, "group", "groups"))
		}
	}
	_, _ = fmt.Fprint(stdout, "\nNext steps:\n"+
		"  1. Review .envguard.yaml: replace the TODO comments with descriptions and check the inferred types.\n"+
		"  2. Run `envguard check` and fix or suppress what it reports.\n"+
		"  3. Commit .envguard.yaml (and .env.example); never commit real env files.\n")
	return nil
}

var errExists = errors.New("file exists")

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	// Project files meant to be committed and read by teammates.
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- shared project file, not a secret
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
