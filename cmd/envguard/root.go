package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/config"
)

// buildInfo describes the binary, as injected at build time.
type buildInfo struct {
	Version string
	Commit  string
	Date    string
}

// globalOptions holds flags shared by every subcommand.
type globalOptions struct {
	Root    string
	Config  string
	NoColor bool
	ASCII   bool
	Verbose bool
}

func newRootCmd(info buildInfo) *cobra.Command {
	opts := &globalOptions{}

	root := &cobra.Command{
		Use:   "envguard",
		Short: "Catch environment-variable and configuration mistakes before you deploy",
		Long: "Your code knows which env vars it needs. EnvGuard makes sure your config agrees — before you deploy.\n\n" +
			"EnvGuard cross-checks source code, .env files, .env templates, Docker Compose files,\n" +
			"GitHub Actions workflows and an optional .envguard.yaml schema. It works offline,\n" +
			"never modifies your files and never prints secret values.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          noArgs,
		Example: "  envguard                       # health report of the current directory\n" +
			"  envguard check                 # short report, exit code 1 on errors (for CI)\n" +
			"  envguard diff staging production\n" +
			"  envguard explain EG003",
	}
	root.SuggestionsMinimumDistance = 2 // cobra only suggests commands when this is set
	// Every flag mistake says where to find the valid flags.
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return fmt.Errorf("%w; run `%s --help` to see the available flags", err, cmd.CommandPath())
	})
	// `envguard` without a command runs doctor.
	do := &doctorOptions{}
	do.addFlags(root)
	root.RunE = func(cmd *cobra.Command, _ []string) error { return runDoctor(cmd, opts, do, info) }

	pf := root.PersistentFlags()
	pf.StringVar(&opts.Root, "root", ".", "repository root to analyze")
	pf.StringVar(&opts.Config, "config", "", "path to the config file (default: <root>/.envguard.yaml or .envguard.yml if present)")
	pf.BoolVar(&opts.NoColor, "no-color", false, "disable colored output (also set by the NO_COLOR environment variable)")
	pf.BoolVar(&opts.ASCII, "ascii", false, "use ASCII symbols instead of Unicode (automatic on terminals that cannot render them)")
	pf.BoolVar(&opts.Verbose, "verbose", false, "print diagnostic information to stderr")

	root.AddCommand(
		newCheckCmd(opts, info),
		newDoctorCmd(opts, info),
		newDiffCmd(opts, info),
		newInitCmd(opts),
		newExplainCmd(opts),
		newVersionCmd(info),
	)
	return root
}

// configFlags converts global options (and flags of cmd) into config.Flags.
// Only flags the user actually set override the config file.
func configFlags(cmd *cobra.Command, opts *globalOptions) config.Flags {
	f := config.Flags{Root: opts.Root, Config: opts.Config}
	if cmd.Flags().Changed("no-color") || os.Getenv("NO_COLOR") != "" {
		noColor := opts.NoColor || os.Getenv("NO_COLOR") != ""
		f.NoColor = &noColor
	}
	if cmd.Flags().Changed("verbose") {
		f.Verbose = &opts.Verbose
	}
	return f
}

func newVersionCmd(info buildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and build date",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "envguard %s\ncommit: %s\nbuilt:  %s\n",
				info.Version, info.Commit, info.Date)
			return err
		},
	}
}

// noArgs rejects arguments for commands that take none. The message says
// what to do and offers the closest command; it never echoes the argument.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if cmd.HasParent() {
		return fmt.Errorf("%s takes no arguments; run `%s --help` for its flags", cmd.Name(), cmd.CommandPath())
	}
	msg := "unknown command"
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += fmt.Sprintf(" (did you mean `envguard %s`?)", suggestions[0])
	}
	return fmt.Errorf("%s; run `envguard --help` to see the commands", msg)
}
