package main

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/report"
)

type doctorOptions struct {
	failOn string
	envs   []string
	app    string
	report reportFlags
}

const doctorLong = "Doctor runs the same analysis as check and prints a friendly health report: what was\n" +
	"scanned, every finding with why it matters and numbered fix steps, the environment matrix\n" +
	"(variables × environments), the secrets and vars your workflows need, and next steps.\n\n" +
	"Doctor never fails (exit 0) unless --fail-on is given; then 1 means findings at or above\n" +
	"that severity. Exit code 2 means a configuration or runtime error. Running `envguard`\n" +
	"without a command runs doctor."

func newDoctorCmd(opts *globalOptions, info buildInfo) *cobra.Command {
	do := &doctorOptions{}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Print a health report of the repository's env setup",
		Long:  doctorLong,
		Example: "  envguard doctor\n" +
			"  envguard doctor --env production\n" +
			"  envguard doctor --format json --output report.json\n" +
			"  envguard doctor --fail-on error                    # exit 1 when there are errors",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd, opts, do, info)
		},
	}
	do.addFlags(cmd)
	return cmd
}

func (do *doctorOptions) addFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&do.failOn, "fail-on", config.FailOnNever, "lowest severity that causes exit code 1: error, warning, info or never")
	f.StringSliceVar(&do.envs, "env", nil, "only report these environments (comma-separated or repeated)")
	f.StringVar(&do.app, "app", "", "app root to report on in a monorepo, relative to --root")
	do.report.add(cmd, report.FormatDoctor)
}

func runDoctor(cmd *cobra.Command, opts *globalOptions, do *doctorOptions, info buildInfo) error {
	started := time.Now()
	failOn := config.FailOnNever
	if cmd.Flags().Changed("fail-on") {
		if err := config.ValidateFailOn(do.failOn); err != nil {
			return err
		}
		failOn = do.failOn
	}
	format, err := resolveFormat(cmd, do.report.format, report.FormatDoctor, report.Formats(), false)
	if err != nil {
		return err
	}
	settings, err := config.Load(configFlags(cmd, opts))
	if err != nil {
		return err
	}
	a, err := analyze(cmd.ErrOrStderr(), settings, analysisOptions{envs: do.envs, app: do.app}, started)
	if err != nil {
		return err
	}
	t := time.Now()
	rep := buildReport(a, "doctor", info.Version, opts, do.envs, do.app)
	if err := emit(cmd.OutOrStdout(), opts, &do.report, format, rep); err != nil {
		return err
	}
	logger{w: cmd.ErrOrStderr(), enabled: settings.Verbose}.phase("report", t)
	return failed(failOn, a.findings)
}
