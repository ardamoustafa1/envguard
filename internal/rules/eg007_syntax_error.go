package rules

import (
	"fmt"

	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/parser"
	"github.com/ardamoustafa1/envguard/internal/project"
)

func init() { Register(syntaxError{base{"EG007", "syntax-error", findings.Error}}) }

// syntaxError turns parser problems and unreadable env files into findings.
// Parser warnings (unquoted whitespace, malformed ignore directives) keep
// warning severity; everything else is an error.
type syntaxError struct{ base }

func (syntaxError) Check(p *project.Project) []findings.Finding {
	out := yamlSyntaxErrors(p)
	for _, f := range p.EnvFiles {
		if !p.EnvSelected(f.Environment) {
			continue
		}
		if f.Parsed == nil {
			out = append(out, findings.Finding{
				Message: fmt.Sprintf("%s could not be read: %s", f.Path, f.ReadErr),
				File:    f.Path,
				Why:     "An unreadable env file cannot be checked, and the app or deploy tooling will probably fail to read it too.",
				Fix:     fmt.Sprintf("Check the permissions of %s, or exclude it in .envguard.yaml if it is not an env file.", f.Path),
			})
			continue
		}
		for _, pr := range f.Parsed.Problems {
			sev := findings.Error
			if pr.Severity == parser.SeverityWarning {
				sev = findings.Warning
			}
			msg := pr.Message
			if pr.Key != "" {
				msg = pr.Key + ": " + msg
			}
			out = append(out, findings.Finding{
				Severity: sev,
				Message:  msg,
				Variable: pr.Key,
				File:     f.Path,
				Line:     pr.Pos.Line,
				Column:   pr.Pos.Column,
				Why:      syntaxWhy(pr.Kind),
				Fix:      pr.Hint,
			})
		}
	}
	return out
}

// yamlSyntaxErrors reports Compose and workflow files that are not valid YAML.
func yamlSyntaxErrors(p *project.Project) []findings.Finding {
	var out []findings.Finding
	add := func(file, what string, line, col int, msg string) {
		out = append(out, findings.Finding{
			Message: fmt.Sprintf("%s: %s", what, msg),
			File:    file,
			Line:    line,
			Column:  col,
			Why: fmt.Sprintf("An invalid %s cannot be checked by envguard, and the tool that reads it will refuse it "+
				"or read something different from what you intended.", what),
			Fix: fmt.Sprintf("Fix the YAML syntax in %s (indentation, unclosed quotes or brackets, tabs instead of spaces).", file),
		})
	}
	for _, cf := range p.ComposeFiles {
		if cf.Problem != nil {
			add(cf.Path, "Compose file", cf.Problem.Line, cf.Problem.Column, cf.Problem.Message)
		}
	}
	for _, wf := range p.WorkflowFiles {
		if wf.Problem != nil {
			add(wf.Path, "workflow", wf.Problem.Line, wf.Problem.Column, wf.Problem.Message)
		}
	}
	return out
}

func syntaxWhy(kind parser.ProblemKind) string {
	switch kind {
	case parser.ProblemUnquotedWhitespace:
		return "Loaders disagree on unquoted values with spaces: some keep the whole line, some stop at the first space, " +
			"and shells treat the rest as a command."
	case parser.ProblemInvalidDirective:
		return "A malformed envguard-ignore directive suppresses nothing, so the findings it was meant to hide still appear."
	case parser.ProblemInvalidEscape:
		return "Loaders handle unknown escape sequences differently, so the value your app receives depends on which loader reads the file."
	case parser.ProblemFileTooLarge:
		return "An env file this large is almost certainly not a hand-written env file, and it was not checked."
	default:
		return "Env loaders skip or misread malformed lines, so the affected variable silently ends up with a wrong value or none at all."
	}
}
