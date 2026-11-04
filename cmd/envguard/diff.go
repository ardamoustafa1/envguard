package main

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/report"
)

type diffOptions struct {
	app      string
	envs     []string
	exitCode bool
	report   reportFlags
}

// errDifferences is returned by `diff --exit-code` when environments differ.
var errDifferences = fmt.Errorf("environments differ: %w", errFindings)

func newDiffCmd(opts *globalOptions, info buildInfo) *cobra.Command {
	do := &diffOptions{}
	cmd := &cobra.Command{
		Use:   "diff [envA envB]",
		Short: "Compare variables between environments",
		Long: "Without arguments, diff prints the variables × environments matrix of every app root:\n" +
			"which environment defines which variable (never the values).\n\n" +
			"With two environments (aliases like prod or dev are fine), it prints only the variables\n" +
			"that one defines and the other does not. An environment without a real env file is\n" +
			"represented by its templates.\n\n" +
			"Exit code 0, or 1 with --exit-code when there are differences (2 for configuration or runtime errors).",
		Example: "  envguard diff\n  envguard diff staging production\n  envguard diff dev prod --format json",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 && len(args) != 2 {
				return errors.New("diff takes no arguments (full matrix) or two environments, e.g. `envguard diff staging production`")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd, opts, do, info, args)
		},
	}
	f := cmd.Flags()
	f.StringVar(&do.app, "app", "", "only this app root (relative to --root)")
	f.StringSliceVar(&do.envs, "env", nil, "matrix only: limit the columns to these environments")
	f.BoolVar(&do.exitCode, "exit-code", false, "exit with 1 if the two environments differ (or the matrix has missing variables)")
	f.StringVar(&do.report.format, "format", report.FormatPretty, "output format: pretty or json")
	f.StringVarP(&do.report.output, "output", "o", "", "write the result to this file instead of stdout")
	return cmd
}

func runDiff(cmd *cobra.Command, opts *globalOptions, do *diffOptions, info buildInfo, args []string) error {
	started := time.Now()
	format, err := resolveFormat(cmd, do.report.format, report.FormatPretty, []string{report.FormatPretty, report.FormatJSON}, false)
	if err != nil {
		return err
	}
	settings, err := config.Load(configFlags(cmd, opts))
	if err != nil {
		return err
	}
	ao := analysisOptions{app: do.app, skipRules: true}
	if len(args) == 0 {
		ao.envs = do.envs
	}
	a, err := analyze(cmd.ErrOrStderr(), settings, ao, started)
	if err != nil {
		return err
	}
	root := opts.Root
	app := appFilter(do.app, a.app)

	var write func(io.Writer, report.Options) error
	differs := false
	if len(args) == 2 {
		d, err := report.BuildDiff(a.project, app, args[0], args[1])
		if err != nil {
			return err
		}
		differs = d.Differences() > 0
		write = func(w io.Writer, o report.Options) error {
			if format == report.FormatJSON {
				return report.DiffJSON(w, info.Version, root, d)
			}
			return report.DiffText(w, d, o)
		}
	} else {
		ms := report.BuildMatrix(a.project, app)
		for _, m := range ms {
			for _, r := range m.Variables {
				for _, st := range r.Cells {
					differs = differs || st == report.CellMissing
				}
			}
		}
		write = func(w io.Writer, o report.Options) error {
			if format == report.FormatJSON {
				return report.MatrixJSON(w, info.Version, root, ms)
			}
			return report.MatrixText(w, ms, o)
		}
	}
	if err := writeOutput(cmd.OutOrStdout(), opts, do.report.output, write); err != nil {
		return err
	}
	if do.exitCode && differs {
		return errDifferences
	}
	return nil
}
