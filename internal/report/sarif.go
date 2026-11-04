package report

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/findings"
)

// SARIF 2.1.0 output, for GitHub code scanning and other SARIF consumers.

const (
	sarifVersion = "2.1.0"
	sarifSchema  = "https://json.schemastore.org/sarif-2.1.0.json"
	// InformationURI is EnvGuard's home page.
	InformationURI = "https://github.com/ardamoustafa1/envguard"
	// fingerprintKey versions the partial fingerprint algorithm.
	fingerprintKey = "envguard/v1"
)

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Invocations []sarifInvocation `json:"invocations"`
	ColumnKind  string            `json:"columnKind"`
	Results     []sarifResult     `json:"results"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Version        string      `json:"version"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifHelp struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown,omitempty"`
}

type sarifRule struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	ShortDescription     sarifText         `json:"shortDescription"`
	FullDescription      sarifText         `json:"fullDescription"`
	HelpURI              string            `json:"helpUri"`
	Help                 sarifHelp         `json:"help"`
	DefaultConfiguration sarifRuleConfig   `json:"defaultConfiguration"`
	Properties           sarifRuleProperty `json:"properties"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifRuleProperty struct {
	Tags []string `json:"tags"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifText         `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	RelatedLocations    []sarifLocation   `json:"relatedLocations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type sarifLocation struct {
	ID               *int                  `json:"id,omitempty"`
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           *sarifRegion  `json:"region,omitempty"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
}

func sarifLevel(s findings.Severity) string {
	switch s {
	case findings.Error:
		return "error"
	case findings.Warning:
		return "warning"
	default:
		return "note"
	}
}

func ruleTags(id string) []string {
	switch id {
	case "EG008", "EG009", "EG010":
		return []string{"security", "configuration"}
	default:
		return []string{"configuration"}
	}
}

// SARIF writes a SARIF 2.1.0 log with one run.
func SARIF(w io.Writer, rep *Report) error {
	driver := sarifDriver{Name: "EnvGuard", Version: rep.Tool.Version, InformationURI: InformationURI, Rules: []sarifRule{}}
	index := map[string]int{}
	for i, r := range rep.Rules {
		index[r.ID] = i
		short := r.Summary
		if short == "" {
			short = r.Name
		}
		full := short
		if why := docSection(r.Markdown, "Why it matters"); why != "" {
			full = short + " " + why
		}
		driver.Rules = append(driver.Rules, sarifRule{
			ID: r.ID, Name: r.Name,
			ShortDescription:     sarifText{Text: short},
			FullDescription:      sarifText{Text: full},
			HelpURI:              r.DocsURL,
			Help:                 sarifHelp{Text: short + " See " + r.DocsURL, Markdown: r.Markdown},
			DefaultConfiguration: sarifRuleConfig{Level: sarifLevel(r.Severity)},
			Properties:           sarifRuleProperty{Tags: ruleTags(r.ID)},
		})
	}
	run := sarifRun{Tool: sarifTool{Driver: driver}, Invocations: []sarifInvocation{{ExecutionSuccessful: true}},
		ColumnKind: "unicodeCodePoints", Results: []sarifResult{}}

	seen := map[string]int{}
	for i := range rep.Findings {
		f := &rep.Findings[i]
		msg := f.Message
		if f.Fix != "" {
			msg += "\n\nWhy: " + f.Why + "\n\nFix: " + f.Fix
		}
		res := sarifResult{
			RuleID: f.RuleID, RuleIndex: index[f.RuleID], Level: sarifLevel(f.Severity),
			Message:   sarifText{Text: msg},
			Locations: []sarifLocation{sarifLoc(rep.PathPrefix, f.Location(), nil)},
		}
		for j, r := range f.Related {
			id := j + 1
			res.RelatedLocations = append(res.RelatedLocations, sarifLoc(rep.PathPrefix, r, &id))
		}
		key := fingerprintBase(f)
		res.PartialFingerprints = map[string]string{fingerprintKey: fingerprint(key, seen[key])}
		seen[key]++
		run.Results = append(run.Results, res)
	}
	return writeJSON(w, sarifLog{Version: sarifVersion, Schema: sarifSchema, Runs: []sarifRun{run}})
}

func sarifLoc(prefix string, l findings.Location, id *int) sarifLocation {
	loc := sarifLocation{ID: id, PhysicalLocation: sarifPhysicalLocation{
		ArtifactLocation: sarifArtifact{URI: uriEscape(prefix + l.File), URIBaseID: "%SRCROOT%"},
	}}
	if l.Line > 0 {
		loc.PhysicalLocation.Region = &sarifRegion{StartLine: l.Line, StartColumn: max(l.Column, 0)}
	}
	return loc
}

// uriEscape percent-encodes the characters that are not allowed in a
// relative URI reference (spaces, '%', '#', '?', non-ASCII).
func uriEscape(p string) string {
	var b strings.Builder
	for _, c := range []byte(p) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte("-._~/!$&'()*+,;=:@", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteString("%" + strings.ToUpper(hex.EncodeToString([]byte{c})))
		}
	}
	return b.String()
}

// fingerprintBase identifies a finding independently of its line, so it
// survives unrelated edits: rule, file and variable (or the message for
// findings without a variable).
func fingerprintBase(f *findings.Finding) string {
	subject := f.Variable
	if subject == "" {
		subject = f.Message
	}
	return f.RuleID + "\x00" + f.File + "\x00" + subject
}

func fingerprint(base string, occurrence int) string {
	sum := sha256.Sum256([]byte(base + "\x00" + strconv.Itoa(occurrence)))
	return hex.EncodeToString(sum[:16])
}

// docSection returns a section of a rule document as one line of prose.
func docSection(md, title string) string {
	var b strings.Builder
	in := false
	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = strings.EqualFold(strings.TrimSpace(line[3:]), title)
			continue
		}
		if in {
			b.WriteString(strings.TrimSpace(line) + " ")
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(b.String()), " "))
}
