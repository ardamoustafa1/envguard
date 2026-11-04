package main

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ardamoustafa1/envguard/internal/report"
)

var shortIDRE = regexp.MustCompile(`^(?i)eg0*(\d{1,3})$`)

func newExplainCmd(opts *globalOptions) *cobra.Command {
	var list bool
	cmd := &cobra.Command{
		Use:   "explain <RULE>",
		Short: "Explain a rule, e.g. `envguard explain EG001`",
		Long: "explain prints the documentation of a rule: what it detects, why it matters, a bad and a\n" +
			"good example, and how to suppress it. RULE is an ID (EG001, eg1) or a rule name\n" +
			"(missing-variable). --list (or no argument) lists every rule.",
		Example: "  envguard explain EG001\n  envguard explain missing-variable\n  envguard explain --list",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return errors.New("explain takes one rule, e.g. `envguard explain EG001`; list all rules with `envguard explain --list`")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if list || len(args) == 0 {
				return writeOutput(cmd.OutOrStdout(), opts, "", func(w io.Writer, o report.Options) error {
					return report.RuleList(w, report.RuleInfos(), o)
				})
			}
			r, err := findRule(args[0])
			if err != nil {
				return err
			}
			return writeOutput(cmd.OutOrStdout(), opts, "", func(w io.Writer, o report.Options) error {
				if err := report.Markdown(w, r.Markdown, o); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "\n%s\n", r.DocsURL)
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&list, "list", false, "list all rules with severity and description")
	return cmd
}

var errUnknownRule = errors.New("unknown rule")

// findRule resolves an ID or name. The error never echoes the argument.
func findRule(arg string) (report.RuleInfo, error) {
	q := strings.TrimSpace(arg)
	if m := shortIDRE.FindStringSubmatch(q); m != nil {
		n, _ := strconv.Atoi(m[1])
		q = fmt.Sprintf("EG%03d", n)
	}
	for _, r := range report.RuleInfos() {
		if strings.EqualFold(r.ID, q) || strings.EqualFold(r.Name, q) {
			if r.Markdown == "" {
				return r, fmt.Errorf("rule %s has no documentation; please report this bug", r.ID)
			}
			return r, nil
		}
	}
	return report.RuleInfo{}, fmt.Errorf("%w; run `envguard explain --list` to see all rules", errUnknownRule)
}
