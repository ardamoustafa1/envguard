package report

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/ardamoustafa1/envguard/docs"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/findings"
	"github.com/ardamoustafa1/envguard/internal/project"
	"github.com/ardamoustafa1/envguard/internal/rules"
)

// SchemaVersion is the version of the JSON output schema (docs/json-output.md).
const SchemaVersion = 1

// Format names accepted by --format.
const (
	FormatPretty = "pretty"
	FormatDoctor = "doctor"
	FormatJSON   = "json"
	FormatSARIF  = "sarif"
	FormatGitHub = "github"
)

// Formats lists every report format.
func Formats() []string {
	return []string{FormatPretty, FormatDoctor, FormatJSON, FormatSARIF, FormatGitHub}
}

// Report is everything a reporter may show. It is built once by Build and
// never contains env values.
type Report struct {
	Tool    Tool
	Command string
	// Root is the --root argument as given, slash-separated.
	Root string
	// PathPrefix is prepended to file paths in CI formats (github, sarif) so
	// they resolve from the repository checkout when --root is a relative
	// subdirectory. Empty for "." and absolute roots.
	PathPrefix string
	// App is the --app filter ("" for none).
	App string
	// Environments is the --env filter (canonical names, nil for all).
	Environments []string
	ProcessEnv   bool

	Stats      Stats
	Findings   []findings.Finding
	Suppressed int
	Matrix     []AppMatrix
	CI         CIInventory
	Notes      []Note
	Rules      []RuleInfo

	HasConfig    bool
	HasSchema    bool
	GitAvailable bool
	GitReason    string
	// Duration of the analysis; printed only with Options.ShowDuration.
	Duration time.Duration
}

// Tool identifies EnvGuard.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Stats describe what was analyzed.
type Stats struct {
	EnvFiles       int             `json:"env_files"`
	Templates      int             `json:"templates"`
	ComposeFiles   int             `json:"compose_files"`
	Workflows      int             `json:"workflows"`
	SourceFiles    int             `json:"source_files"`
	Languages      []LanguageCount `json:"languages"`
	Variables      int             `json:"variables"`
	AppRoots       []string        `json:"app_roots"`
	DynamicLookups int             `json:"dynamic_lookups"`
}

// Files is the total number of analyzed files.
func (s Stats) Files() int {
	return s.EnvFiles + s.Templates + s.ComposeFiles + s.Workflows + s.SourceFiles
}

// LanguageCount is the number of scanned source files of one language.
type LanguageCount struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
}

// CIInventory lists what GitHub Actions workflows need.
type CIInventory struct {
	Secrets []CIItem `json:"secrets"`
	Vars    []CIItem `json:"vars"`
}

// CIItem is one secret or variable with the places that reference it.
type CIItem struct {
	Name string              `json:"name"`
	Refs []findings.Location `json:"-"`
}

// Note is an informational scanner remark about one code usage.
type Note struct {
	findings.Location
	Variable string
	Text     string
}

// RuleInfo is the metadata of one rule.
type RuleInfo struct {
	ID       string
	Name     string
	Severity findings.Severity
	// SeverityText is the human description of the default severity from
	// the docs ("error (info when ...)").
	SeverityText string
	Summary      string
	DocsURL      string
	Markdown     string
}

// Meta is the command-level context of a report.
type Meta struct {
	Command      string
	Version      string
	Root         string
	App          string
	Environments []string
	Suppressed   int
	Duration     time.Duration
}

// Options control rendering.
type Options struct {
	// Color enables ANSI colors.
	Color bool
	// ASCII replaces Unicode symbols with ASCII ones.
	ASCII bool
	// Width is the wrap width for prose (doctor, explain). 0 means 80.
	Width int
	// ShowDuration prints the run duration in the pretty summary. It is
	// only set for interactive terminals so piped output stays deterministic.
	ShowDuration bool
}

func (o Options) width() int {
	if o.Width <= 0 {
		return 80
	}
	return min(max(o.Width, 50), 100)
}

// Write renders rep in the given format.
func Write(w io.Writer, format string, rep *Report, opts Options) error {
	switch format {
	case FormatPretty:
		return Pretty(w, rep, opts)
	case FormatDoctor:
		return Doctor(w, rep, opts)
	case FormatJSON:
		return JSON(w, rep)
	case FormatSARIF:
		return SARIF(w, rep)
	case FormatGitHub:
		return GitHub(w, rep)
	default:
		return fmt.Errorf("unknown format: want one of %s", strings.Join(Formats(), ", "))
	}
}

// RuleInfos returns the metadata of every registered rule.
func RuleInfos() []RuleInfo {
	var out []RuleInfo
	for _, r := range rules.All() {
		ri := RuleInfo{ID: r.ID(), Name: r.Name(), Severity: r.DefaultSeverity(), DocsURL: rules.DocsURL(r),
			SeverityText: r.DefaultSeverity().String()}
		if d, ok := docs.Rule(r.ID()); ok {
			ri.Summary, ri.Markdown = d.Summary, d.Markdown
			if d.Severity != "" {
				ri.SeverityText = d.Severity
			}
		}
		out = append(out, ri)
	}
	return out
}

// Build assembles a report from an analyzed project and its findings.
func Build(p *project.Project, fs []findings.Finding, meta Meta) *Report {
	rep := &Report{
		Tool:         Tool{Name: "envguard", Version: meta.Version},
		Command:      meta.Command,
		Root:         meta.Root,
		PathPrefix:   pathPrefix(meta.Root),
		App:          meta.App,
		Environments: meta.Environments,
		ProcessEnv:   p.ProcessEnv,
		Findings:     fs,
		Suppressed:   meta.Suppressed,
		Duration:     meta.Duration,
		Rules:        RuleInfos(),
		HasConfig:    p.Settings != nil && p.Settings.ConfigPath != "",
		HasSchema:    p.Schema != nil,
		GitAvailable: p.Git != nil && p.Git.Available,
	}
	if p.Git != nil {
		rep.GitReason = p.Git.Reason
	}
	if rep.Findings == nil {
		rep.Findings = []findings.Finding{}
	}
	rep.Stats = buildStats(p)
	rep.Matrix = BuildMatrix(p, meta.App)
	rep.CI = CIInventory{Secrets: ciItems(p.CI.Secrets), Vars: ciItems(p.CI.Vars)}
	for _, u := range p.UsagesUnder(appOrAll(meta.App)) {
		if u.Note != "" {
			rep.Notes = append(rep.Notes, Note{
				Location: findings.Location{File: u.File, Line: u.Line, Column: u.Column},
				Variable: u.Variable, Text: u.Note,
			})
		}
	}
	return rep
}

func appOrAll(app string) string {
	if app == "" {
		return "."
	}
	return app
}

func pathPrefix(root string) string {
	r := strings.TrimPrefix(strings.ReplaceAll(root, "\\", "/"), "./")
	r = strings.TrimSuffix(r, "/")
	if r == "" || r == "." || strings.HasPrefix(r, "/") || strings.HasPrefix(r, "..") ||
		(len(r) >= 2 && r[1] == ':') {
		return ""
	}
	return r + "/"
}

func ciItems(in []project.CIItem) []CIItem {
	out := []CIItem{}
	for _, it := range in {
		ci := CIItem{Name: it.Name}
		for _, r := range it.Refs {
			ci.Refs = append(ci.Refs, findings.Location{File: r.File, Line: r.Line, Column: r.Column})
		}
		out = append(out, ci)
	}
	return out
}

func buildStats(p *project.Project) Stats {
	s := Stats{ComposeFiles: len(p.ComposeFiles), Workflows: len(p.WorkflowFiles), Languages: []LanguageCount{},
		AppRoots: []string{}}
	names := map[string]bool{}
	for _, f := range p.EnvFiles {
		if f.IsTemplate() {
			s.Templates++
		} else if f.Path != project.ProcessEnvPath {
			s.EnvFiles++
		}
		if f.Path == project.ProcessEnvPath {
			continue
		}
		for _, k := range f.Keys() {
			names[k] = true
		}
	}
	if !p.ProcessEnv && p.Inventory != nil {
		unreadable := map[string]bool{}
		for _, f := range p.UnscannableFiles {
			unreadable[f] = true
		}
		langs := map[string]int{}
		for _, f := range p.Inventory.ByKind(discovery.KindSource) {
			if !unreadable[f.Path] {
				langs[string(f.Language)]++
				s.SourceFiles++
			}
		}
		for l, n := range langs {
			s.Languages = append(s.Languages, LanguageCount{Language: l, Files: n})
		}
		slices.SortFunc(s.Languages, func(a, b LanguageCount) int {
			return cmp.Or(cmp.Compare(b.Files, a.Files), strings.Compare(a.Language, b.Language))
		})
	}
	for _, u := range p.CodeUsages {
		if u.Certain() {
			names[u.Variable] = true
		}
	}
	for _, r := range p.ComposeRefs {
		names[r.Variable] = true
	}
	for _, r := range p.WorkflowRefs {
		names[r.Variable] = true
	}
	if p.Schema != nil {
		for _, n := range p.Schema.Names() {
			names[n] = true
		}
	}
	s.Variables = len(names)
	s.DynamicLookups = p.DynamicUsages(".")
	if p.Inventory != nil {
		s.AppRoots = append(s.AppRoots, p.Inventory.AppRoots...)
	}
	if !slices.Contains(s.AppRoots, ".") {
		s.AppRoots = append([]string{"."}, s.AppRoots...)
	}
	slices.Sort(s.AppRoots)
	return s
}

// bySeverity returns findings ordered by severity (errors first), keeping
// the deterministic file order within a severity.
func bySeverity(fs []findings.Finding) []findings.Finding {
	out := slices.Clone(fs)
	slices.SortStableFunc(out, func(a, b findings.Finding) int { return cmp.Compare(b.Severity, a.Severity) })
	return out
}
