package report

import (
	"io"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// jsonReport is the stable JSON document (schema_version 1). See
// docs/json-output.md; changing a field name or meaning requires a new
// schema version.
type jsonReport struct {
	SchemaVersion int           `json:"schema_version"`
	Tool          Tool          `json:"tool"`
	Command       string        `json:"command"`
	Root          string        `json:"root"`
	ProcessEnv    bool          `json:"process_env"`
	Stats         Stats         `json:"stats"`
	Summary       jsonSummary   `json:"summary"`
	Findings      []jsonFinding `json:"findings"`
	Matrix        []AppMatrix   `json:"matrix"`
	CI            jsonCI        `json:"ci"`
	Notes         []jsonNote    `json:"notes"`
}

type jsonSummary struct {
	Errors     int `json:"errors"`
	Warnings   int `json:"warnings"`
	Info       int `json:"info"`
	Suppressed int `json:"suppressed"`
}

type jsonLocation struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type jsonFinding struct {
	RuleID   string         `json:"rule_id"`
	RuleName string         `json:"rule_name"`
	Severity string         `json:"severity"`
	Message  string         `json:"message"`
	Variable string         `json:"variable"`
	Location jsonLocation   `json:"location"`
	Evidence string         `json:"evidence"`
	Why      string         `json:"why"`
	Fix      string         `json:"fix"`
	Related  []jsonLocation `json:"related"`
	DocsURL  string         `json:"docs_url"`
}

type jsonCIItem struct {
	Name string         `json:"name"`
	Refs []jsonLocation `json:"refs"`
}

type jsonCI struct {
	Secrets []jsonCIItem `json:"secrets"`
	Vars    []jsonCIItem `json:"vars"`
}

type jsonNote struct {
	Location jsonLocation `json:"location"`
	Variable string       `json:"variable"`
	Text     string       `json:"text"`
}

func jsonLoc(l findings.Location) jsonLocation {
	return jsonLocation{File: l.File, Line: l.Line, Column: l.Column}
}

// JSON writes the report as an indented JSON document.
func JSON(w io.Writer, rep *Report) error {
	c := findings.Count(rep.Findings)
	out := jsonReport{
		SchemaVersion: SchemaVersion,
		Tool:          rep.Tool,
		Command:       rep.Command,
		Root:          rep.Root,
		ProcessEnv:    rep.ProcessEnv,
		Stats:         rep.Stats,
		Summary:       jsonSummary{Errors: c.Error, Warnings: c.Warning, Info: c.Info, Suppressed: rep.Suppressed},
		Findings:      []jsonFinding{},
		Matrix:        rep.Matrix,
		CI:            jsonCI{Secrets: jsonCIItems(rep.CI.Secrets), Vars: jsonCIItems(rep.CI.Vars)},
		Notes:         []jsonNote{},
	}
	if out.Matrix == nil {
		out.Matrix = []AppMatrix{}
	}
	for i := range rep.Findings {
		f := &rep.Findings[i]
		jf := jsonFinding{
			RuleID: f.RuleID, RuleName: f.RuleName, Severity: f.Severity.String(), Message: f.Message,
			Variable: f.Variable, Location: jsonLoc(f.Location()), Evidence: f.Evidence, Why: f.Why, Fix: f.Fix,
			Related: []jsonLocation{}, DocsURL: f.DocsURL,
		}
		for _, r := range f.Related {
			jf.Related = append(jf.Related, jsonLoc(r))
		}
		out.Findings = append(out.Findings, jf)
	}
	for _, n := range rep.Notes {
		out.Notes = append(out.Notes, jsonNote{Location: jsonLoc(n.Location), Variable: n.Variable, Text: n.Text})
	}
	return writeJSON(w, out)
}

func jsonCIItems(in []CIItem) []jsonCIItem {
	out := []jsonCIItem{}
	for _, it := range in {
		j := jsonCIItem{Name: it.Name, Refs: []jsonLocation{}}
		for _, r := range it.Refs {
			j.Refs = append(j.Refs, jsonLoc(r))
		}
		out = append(out, j)
	}
	return out
}
