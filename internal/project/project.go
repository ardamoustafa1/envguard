package project

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardamoustafa1/envguard/internal/compose"
	"github.com/ardamoustafa1/envguard/internal/config"
	"github.com/ardamoustafa1/envguard/internal/discovery"
	"github.com/ardamoustafa1/envguard/internal/gitutil"
	"github.com/ardamoustafa1/envguard/internal/mask"
	"github.com/ardamoustafa1/envguard/internal/parser"
	"github.com/ardamoustafa1/envguard/internal/scanner"
	"github.com/ardamoustafa1/envguard/internal/schema"
	"github.com/ardamoustafa1/envguard/internal/workflow"
)

// Project is the immutable model every rule receives. It is built once by
// Build; all I/O happens there. Rules must treat every field as read-only.
type Project struct {
	// Root is the absolute root directory.
	Root string
	// Settings is the effective configuration.
	Settings *config.Settings
	// Inventory is the discovery result.
	Inventory *discovery.Inventory
	// Apps are the app roots that contain env files or templates, sorted by
	// root ("." first).
	Apps []*App
	// EnvFiles lists every env file and template, sorted by path.
	EnvFiles []*EnvFile

	// CodeUsages are env var reads found in source code by the scanner,
	// sorted by file and position (dynamic lookups included).
	CodeUsages []CodeUsage
	// ScannedFiles is the number of source files the scanner read;
	// UnscannableFiles lists those it could not read.
	ScannedFiles     int
	UnscannableFiles []string

	// ComposeFiles and WorkflowFiles are the parsed Compose files and GitHub
	// Actions workflows; ComposeRefs and WorkflowRefs flatten their
	// variables for rules.
	ComposeFiles  []*compose.File
	WorkflowFiles []*workflow.File
	ComposeRefs   []ComposeRef
	WorkflowRefs  []WorkflowRef
	// CI lists the secrets and vars the workflows need (for doctor).
	CI CIInventory
	// Git is never nil after Build; check Git.Available before using it.
	Git *GitStatus

	// Schema is the variables: section of the config, nil when empty.
	Schema *Schema
	// ProcessEnv is true in --process-env mode (see Options.ProcessEnv).
	ProcessEnv bool

	envFilter map[string]bool
	byPath    map[string]*EnvFile
}

// App groups the env files of one app root.
type App struct {
	// Root is the app root directory, relative and slash-separated ("." for
	// the repository root).
	Root string
	// Groups holds one group per environment: "default" first, then the
	// others sorted by name.
	Groups []*EnvGroup
}

// EnvGroup holds the real env files and templates of one environment.
type EnvGroup struct {
	Environment string
	Files       []*EnvFile
	Templates   []*EnvFile
}

// EnvFile is a discovered and parsed env file or template.
type EnvFile struct {
	discovery.File
	// Parsed is nil if the file could not be read (see ReadErr).
	Parsed *parser.File
	// ReadErr describes an I/O failure; it never contains file content.
	ReadErr string
}

// IsTemplate reports whether f is a template.
func (f *EnvFile) IsTemplate() bool { return f.Kind == discovery.KindEnvTemplate }

// Entries returns the parsed entries, or nil if the file was unreadable.
func (f *EnvFile) Entries() []parser.Entry {
	if f.Parsed == nil {
		return nil
	}
	return f.Parsed.Entries
}

// Lookup returns the first entry with the given key.
func (f *EnvFile) Lookup(key string) (parser.Entry, bool) {
	for _, e := range f.Entries() {
		if e.Key == key {
			return e, true
		}
	}
	return parser.Entry{}, false
}

// Keys returns the distinct keys of the file in first-appearance order.
func (f *EnvFile) Keys() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range f.Entries() {
		if !seen[e.Key] {
			seen[e.Key] = true
			out = append(out, e.Key)
		}
	}
	return out
}

// CodeUsage is an environment variable read in source code (see scanner.Usage).
type CodeUsage = scanner.Usage

// ComposeRef is a variable a Compose file defines (under environment:) or
// interpolates (${VAR}).
type ComposeRef struct {
	Variable   string
	File       string
	Line       int
	Column     int
	HasDefault bool
	Defines    bool
	Service    string // set for definitions
}

// WorkflowRef is an env: key (Defines) or a secrets./vars./env. reference in
// a GitHub Actions workflow.
type WorkflowRef struct {
	Variable string
	Context  string // "secrets", "vars" or "env"
	Defines  bool
	File     string
	Line     int
	Column   int
}

// GitStatus is git's view of the root (see gitutil.Status).
type GitStatus = gitutil.Status

// Location is a position in a file (1-based; 0 = not applicable).
type Location struct {
	File         string
	Line, Column int
}

// CIItem is a secret or variable the workflows reference.
type CIItem struct {
	Name string
	// Refs are the reference positions, in file order.
	Refs []Location
}

// CIInventory lists what must be configured in GitHub for the workflows to
// run: repository/environment secrets and configuration variables, sorted
// by name. GITHUB_TOKEN is never listed.
type CIInventory struct {
	Secrets []CIItem
	Vars    []CIItem
}

// Schema holds the validated variable schema (filled by internal/schema).
type Schema struct {
	// File is the config file relative to the root (or as given when it
	// lies outside the root).
	File      string
	Variables map[string]*SchemaVar
}

// SchemaVar is one entry of the variables: section.
type SchemaVar struct {
	Name   string
	Config config.Variable
	Spec   schema.Spec
	Line   int
	Column int
}

// Location returns the position of the variable in the config file.
func (s *Schema) Location(name string) Location {
	v := s.Variables[name]
	if v == nil {
		return Location{File: s.File}
	}
	return Location{File: s.File, Line: v.Line, Column: v.Column}
}

// Names returns the schema variable names, sorted.
func (s *Schema) Names() []string {
	out := make([]string, 0, len(s.Variables))
	for n := range s.Variables {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// ProcessEnvPath is the pseudo file name of the process environment in
// --process-env mode.
const ProcessEnvPath = "(process environment)"

// ProcessEnv selects --process-env mode: the given variables stand in for
// the real env files of one environment of one app root.
type ProcessEnv struct {
	Vars map[string]string
	// Env is the environment name ("" means default).
	Env string
	// App is the app root ("" means ".").
	App string
}

// Options configures Build.
type Options struct {
	// Environments limits which environments rules check (empty: all).
	// Names are normalized (prod → production).
	Environments []string
	// ProcessEnv, if set, switches to --process-env mode: real env files are
	// ignored and replaced by one synthetic file holding these variables;
	// the scanner, Compose, workflows and git are skipped.
	ProcessEnv *ProcessEnv
	// EnvFilesOnly reads only env files and templates: the scanner, Compose,
	// workflows and git are skipped. For callers that only compare
	// environments (diff), which on a large repository saves nearly all the
	// time.
	EnvFilesOnly bool
}

// Build reads and parses every env file and template in inv and assembles
// the Project. Unreadable files are recorded on their EnvFile, not returned
// as errors.
func Build(settings *config.Settings, inv *discovery.Inventory, opts Options) (*Project, error) {
	if settings == nil || inv == nil {
		return nil, errors.New("project: settings and inventory are required")
	}
	p := &Project{
		Root:      inv.Root,
		Settings:  settings,
		Inventory: inv,
		byPath:    map[string]*EnvFile{},
	}
	if len(opts.Environments) > 0 {
		p.envFilter = map[string]bool{}
		for _, e := range opts.Environments {
			p.envFilter[discovery.NormalizeEnvironment(e)] = true
		}
	}

	p.Schema = buildSchema(settings, inv.Root)

	apps := map[string]*App{}
	for _, df := range inv.Files {
		if df.Kind != discovery.KindEnv && df.Kind != discovery.KindEnvTemplate {
			continue
		}
		if opts.ProcessEnv != nil && df.Kind == discovery.KindEnv {
			continue // real env files are replaced by the process environment
		}
		ef := &EnvFile{File: df}
		ef.Parsed, ef.ReadErr = parseFile(inv.Root, df.Path)
		p.EnvFiles = append(p.EnvFiles, ef)
		p.byPath[df.Path] = ef

		app := apps[df.AppRoot]
		if app == nil {
			app = &App{Root: df.AppRoot}
			apps[df.AppRoot] = app
		}
		g := app.Group(df.Environment)
		if g == nil {
			g = &EnvGroup{Environment: df.Environment}
			app.Groups = append(app.Groups, g)
		}
		if ef.IsTemplate() {
			g.Templates = append(g.Templates, ef)
		} else {
			g.Files = append(g.Files, ef)
		}
	}

	if pe := opts.ProcessEnv; pe != nil {
		ef := processEnvFile(pe)
		p.EnvFiles = append(p.EnvFiles, ef)
		p.byPath[ef.Path] = ef
		app := apps[ef.AppRoot]
		if app == nil {
			app = &App{Root: ef.AppRoot}
			apps[ef.AppRoot] = app
		}
		g := app.Group(ef.Environment)
		if g == nil {
			g = &EnvGroup{Environment: ef.Environment}
			app.Groups = append(app.Groups, g)
		}
		g.Files = append(g.Files, ef)
		p.envFilter = map[string]bool{ef.Environment: true}
		p.ProcessEnv = true
	}

	for _, app := range apps {
		slices.SortFunc(app.Groups, func(a, b *EnvGroup) int {
			if a.Environment == b.Environment {
				return 0
			}
			if a.Environment == discovery.EnvDefault {
				return -1
			}
			if b.Environment == discovery.EnvDefault {
				return 1
			}
			return strings.Compare(a.Environment, b.Environment)
		})
		p.Apps = append(p.Apps, app)
	}
	slices.SortFunc(p.Apps, func(a, b *App) int { return cmp.Compare(appSortKey(a.Root), appSortKey(b.Root)) })

	if opts.EnvFilesOnly {
		p.Git = &GitStatus{Reason: "not needed to compare environments"}
		slices.SortFunc(p.EnvFiles, func(a, b *EnvFile) int { return strings.Compare(a.Path, b.Path) })
		return p, nil
	}
	if opts.ProcessEnv != nil {
		p.Git = &GitStatus{Reason: "not used in --process-env mode"}
		slices.SortFunc(p.EnvFiles, func(a, b *EnvFile) int { return strings.Compare(a.Path, b.Path) })
		return p, nil
	}

	scan := scanner.Scan(scanner.Options{Root: inv.Root, Files: inv.Files})
	p.CodeUsages = scan.Usages
	p.ScannedFiles = scan.Scanned
	p.UnscannableFiles = scan.Unreadable

	p.loadCompose()
	p.loadWorkflows()

	paths := make([]string, len(p.EnvFiles))
	for i, f := range p.EnvFiles {
		paths[i] = f.Path
	}
	p.Git = gitutil.Inspect(context.Background(), inv.Root, paths)
	slices.SortFunc(p.EnvFiles, func(a, b *EnvFile) int { return strings.Compare(a.Path, b.Path) })
	return p, nil
}

func (p *Project) loadCompose() {
	for _, df := range p.Inventory.ByKind(discovery.KindCompose) {
		data, err := readFile(p.Root, df.Path)
		if err != nil {
			continue // unreadable files were already reported by discovery
		}
		cf := compose.Parse(data, df.Path)
		for si := range cf.Services {
			svc := &cf.Services[si]
			for ei := range svc.EnvFiles {
				ef := &svc.EnvFiles[ei]
				if ef.Path == "" {
					ef.Exists = true // outside the root: cannot check, assume present
					continue
				}
				_, statErr := os.Stat(filepath.Join(p.Root, filepath.FromSlash(ef.Path)))
				ef.Exists = statErr == nil
			}
			for _, ev := range svc.Environment {
				p.ComposeRefs = append(p.ComposeRefs, ComposeRef{Variable: ev.Name, File: cf.Path,
					Line: ev.Line, Column: ev.Column, Defines: true, Service: svc.Name})
			}
		}
		for _, it := range cf.Interpolations {
			p.ComposeRefs = append(p.ComposeRefs, ComposeRef{Variable: it.Variable, File: cf.Path,
				Line: it.Line, Column: it.Column, HasDefault: it.HasDefault})
		}
		p.ComposeFiles = append(p.ComposeFiles, cf)
	}
}

func (p *Project) loadWorkflows() {
	secrets := map[string][]Location{}
	vars := map[string][]Location{}
	for _, df := range p.Inventory.ByKind(discovery.KindWorkflow) {
		data, err := readFile(p.Root, df.Path)
		if err != nil {
			continue
		}
		wf := workflow.Parse(data, df.Path)
		for _, r := range wf.Refs {
			p.WorkflowRefs = append(p.WorkflowRefs, WorkflowRef{Variable: r.Name, Context: r.Context,
				Defines: r.Define, File: wf.Path, Line: r.Line, Column: r.Column})
			loc := Location{File: wf.Path, Line: r.Line, Column: r.Column}
			switch {
			case r.Define:
			case r.Context == workflow.ContextSecrets:
				secrets[r.Name] = append(secrets[r.Name], loc)
			case r.Context == workflow.ContextVars:
				vars[r.Name] = append(vars[r.Name], loc)
			}
		}
		p.WorkflowFiles = append(p.WorkflowFiles, wf)
	}
	p.CI.Secrets = ciItems(secrets)
	p.CI.Vars = ciItems(vars)
}

func ciItems(m map[string][]Location) []CIItem {
	out := make([]CIItem, 0, len(m))
	for name, refs := range m {
		out = append(out, CIItem{Name: name, Refs: refs})
	}
	slices.SortFunc(out, func(a, b CIItem) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func buildSchema(settings *config.Settings, root string) *Schema {
	if len(settings.Variables) == 0 {
		return nil
	}
	s := &Schema{File: settings.ConfigPath, Variables: map[string]*SchemaVar{}}
	// The root is symlink-resolved by discovery; resolve the config path the
	// same way so it can be expressed relative to the root.
	if abs, err := filepath.Abs(filepath.FromSlash(settings.ConfigPath)); err == nil {
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			s.File = filepath.ToSlash(rel)
		}
	}
	for name, v := range settings.Variables {
		pos := settings.VariablePositions[name]
		s.Variables[name] = &SchemaVar{Name: name, Config: v, Spec: v.Spec(), Line: pos.Line, Column: pos.Column}
	}
	return s
}

// processEnvFile turns process variables into a synthetic real env file.
// Entries are sorted by name; positions are zero (no file to point into).
func processEnvFile(pe *ProcessEnv) *EnvFile {
	env := discovery.EnvDefault
	if pe.Env != "" {
		env = discovery.NormalizeEnvironment(pe.Env)
	}
	app := pe.App
	if app == "" {
		app = "."
	}
	names := make([]string, 0, len(pe.Vars))
	for k := range pe.Vars {
		names = append(names, k)
	}
	slices.Sort(names)
	parsed := &parser.File{Filename: ProcessEnvPath}
	for _, k := range names {
		parsed.Entries = append(parsed.Entries, parser.Entry{Key: k, Value: pe.Vars[k], RawValue: pe.Vars[k], Quote: parser.QuoteNone})
	}
	return &EnvFile{
		File:   discovery.File{Path: ProcessEnvPath, Kind: discovery.KindEnv, Environment: env, AppRoot: app},
		Parsed: parsed,
	}
}

// SchemaVar returns the schema entry for name, or nil.
func (p *Project) SchemaVar(name string) *SchemaVar {
	if p.Schema == nil {
		return nil
	}
	return p.Schema.Variables[name]
}

// Sensitive reports whether a value of name must never be shown: the
// schema marks it sensitive or the name looks sensitive.
func (p *Project) Sensitive(name string) bool {
	if sv := p.SchemaVar(name); sv != nil && sv.Config.Sensitive {
		return true
	}
	return mask.IsSensitiveName(name)
}

// DisplayValue is mask.Display plus the schema's `sensitive: true`, which
// forces masking even for harmless-looking names. Rules must use it (or
// mask directly) for every value they put into output.
func (p *Project) DisplayValue(name, value string) string {
	if sv := p.SchemaVar(name); sv != nil && sv.Config.Sensitive {
		return mask.Placeholder(value)
	}
	return mask.Display(name, value)
}

func readFile(root, rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) // #nosec G304 -- rel comes from discovery inside root
}

func appSortKey(root string) string {
	if root == "." {
		return ""
	}
	return root
}

func parseFile(root, rel string) (parsed *parser.File, readErr string) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel))) // #nosec G304 -- rel comes from discovery inside root
	if err != nil {
		return nil, describe(err)
	}
	defer func() { _ = f.Close() }()
	parsed, err = parser.Parse(f, rel)
	if err != nil {
		return nil, describe(err)
	}
	return parsed, ""
}

// describe returns an error description without paths or content.
func describe(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// Group returns the group for an environment, or nil.
func (a *App) Group(env string) *EnvGroup {
	for _, g := range a.Groups {
		if g.Environment == env {
			return g
		}
	}
	return nil
}

// Templates returns all templates of the app, in group order.
func (a *App) Templates() []*EnvFile {
	var out []*EnvFile
	for _, g := range a.Groups {
		out = append(out, g.Templates...)
	}
	return out
}

// ApplicableTemplates returns the templates that document an environment:
// the default templates followed by that environment's own templates.
// Local templates (.env.local.example) apply only to local files.
func (a *App) ApplicableTemplates(env string, local bool) []*EnvFile {
	var out []*EnvFile
	add := func(g *EnvGroup) {
		if g == nil {
			return
		}
		for _, t := range g.Templates {
			if !t.Local || local {
				out = append(out, t)
			}
		}
	}
	add(a.Group(discovery.EnvDefault))
	if env != discovery.EnvDefault {
		add(a.Group(env))
	}
	return out
}

// File returns the env file at a relative path, or nil.
func (p *Project) File(rel string) *EnvFile { return p.byPath[rel] }

// EnvSelected reports whether rules should check an environment (--env).
func (p *Project) EnvSelected(env string) bool {
	return p.envFilter == nil || p.envFilter[env]
}

// AppFor returns the app with the given root, or nil.
func (p *Project) AppFor(root string) *App {
	for _, a := range p.Apps {
		if a.Root == root {
			return a
		}
	}
	return nil
}

// AncestorApps returns the apps whose root contains root (excluding root
// itself), nearest first. "." contains everything.
func (p *Project) AncestorApps(root string) []*App {
	var out []*App
	for dir := root; dir != "."; {
		dir = path.Dir(dir)
		if a := p.AppFor(dir); a != nil {
			out = append(out, a)
		}
	}
	return out
}

// IsIgnored reports whether an inline directive in the env file at loc.File
// suppresses ruleID at loc.Line.
func (p *Project) IsIgnored(ruleID, file string, line int) bool {
	f := p.byPath[file]
	if f == nil || f.Parsed == nil {
		return false
	}
	return f.Parsed.IsIgnored(ruleID, line)
}

// UsagesUnder returns the code usages whose app root is root or lies below
// it ("." covers everything), including dynamic ones.
func (p *Project) UsagesUnder(root string) []CodeUsage {
	var out []CodeUsage
	for _, u := range p.CodeUsages {
		if root == "." || u.AppRoot == root || strings.HasPrefix(u.AppRoot, root+"/") {
			out = append(out, u)
		}
	}
	return out
}

// DynamicUsages counts computed-key lookups (process.env[name]) under root
// that could not be analyzed.
func (p *Project) DynamicUsages(root string) int {
	n := 0
	for _, u := range p.UsagesUnder(root) {
		if u.Dynamic {
			n++
		}
	}
	return n
}

// String describes the project briefly (for verbose logs).
func (p *Project) String() string {
	return fmt.Sprintf("%d env files in %d app roots; %d code usages in %d source files; %d compose files; %d workflows",
		len(p.EnvFiles), len(p.Apps), len(p.CodeUsages), p.ScannedFiles, len(p.ComposeFiles), len(p.WorkflowFiles))
}
