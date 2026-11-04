package main

import (
	"errors"
	"time"

	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/report"
)

type checkOptions struct {
	failOn     string
	envs       []string
	quiet      bool
	processEnv bool
	app        string
	report     reportFlags
}

func newCheckCmd(opts *globalOptions, info buildInfo) *cobra.Command {
	co := &checkOptions{}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check the repository for env var and config mistakes",
		Long: "Check discovers env files, templates, code, Compose files and workflows under --root,\n" +
			"runs every rule and prints the findings grouped by file, followed by a summary.\n\n" +
			"Inside GitHub Actions (GITHUB_ACTIONS=true) the default format is github: findings become\n" +
			"pull request annotations and a job summary is written to $GITHUB_STEP_SUMMARY.\n\n" +
			"With --process-env it validates the current process environment instead of env files\n" +
			"(for Docker entrypoints and CI), against the templates and schema of one environment:\n\n" +
			"  envguard check --process-env --env production --quiet && exec \"$@\"\n\n" +
			"Exit codes: 0 = no findings at or above --fail-on, 1 = findings at or above --fail-on,\n" +
			"2 = configuration or runtime error.",
		Example: "  envguard check\n" +
			"  envguard check --env production --fail-on warning\n" +
			"  envguard check --app apps/api                      # one app of a monorepo\n" +
			"  envguard check --format sarif --output envguard.sarif\n" +
			"  envguard check --process-env --env production --quiet && exec \"$@\"",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			started := time.Now()
			flags := configFlags(cmd, opts)
			if cmd.Flags().Changed("fail-on") {
				if err := config.ValidateFailOn(co.failOn); err != nil {
					return err
				}
				flags.FailOn = &co.failOn
			}
			format, err := resolveFormat(cmd, co.report.format, report.FormatPretty, report.Formats(), true)
			if err != nil {
				return err
			}
			if co.processEnv && len(co.envs) > 1 {
				return errors.New("--process-env checks exactly one environment; pass a single --env")
			}
			settings, err := config.Load(flags)
			if err != nil {
				return err
			}
			a, err := analyze(cmd.ErrOrStderr(), settings, analysisOptions{envs: co.envs, app: co.app, processEnv: co.processEnv}, started)
			if err != nil {
				return err
			}
			if !co.quiet {
				t := time.Now()
				rep := buildReport(a, "check", info.Version, opts, co.envs, co.app)
				if err := emit(cmd.OutOrStdout(), opts, &co.report, format, rep); err != nil {
					return err
				}
				logger{w: cmd.ErrOrStderr(), enabled: settings.Verbose}.phase("report", t)
			}
			return failed(settings.FailOn, a.findings)
		},
	}
	f := cmd.Flags()
	f.StringVar(&co.failOn, "fail-on", config.SeverityError, "lowest severity that causes exit code 1: error, warning, info or never")
	f.StringSliceVar(&co.envs, "env", nil, "only check these environments (comma-separated or repeated), e.g. --env production,staging")
	f.BoolVar(&co.quiet, "quiet", false, "print nothing; report the result through the exit code only")
	f.BoolVar(&co.processEnv, "process-env", false,
		"validate the current process environment (instead of env files) against the templates and schema of --env")
	f.StringVar(&co.app, "app", "", "app root to check in a monorepo, relative to --root (e.g. apps/api)")
	co.report.add(cmd, report.FormatPretty)
	return cmd
}
