package compose

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ardamoustafa1/envguard/internal/yamlutil"
)

// File is a parsed Compose file. It never stores values: environment
// entries keep only names, interpolations only variable names.
type File struct {
	// Path is relative to the root, slash-separated.
	Path string
	// Dir is the project directory Compose uses: the directory of the file.
	// Interpolation reads Dir/.env (AdjacentEnv).
	Dir      string
	Services []Service
	// Interpolations are every ${VAR}, ${VAR:-d}, ${VAR-d}, ${VAR:?e},
	// ${VAR?e}, ${VAR:+a} and $VAR anywhere in the file; $$ is a literal $.
	Interpolations []Interpolation
	// Problem is set when the file is not valid YAML.
	Problem *yamlutil.Problem
}

// AdjacentEnv is the .env file Compose reads for interpolation.
func (f *File) AdjacentEnv() string { return joinRel(f.Dir, ".env") }

// Service is one entry under `services:`.
type Service struct {
	Name         string
	Line, Column int
	Environment  []EnvVar
	EnvFiles     []EnvFile
}

// EnvVar is a name under `environment:` (map or list form).
type EnvVar struct {
	Name         string
	Line, Column int
	// PassThrough is true when no value is given (`- KEY` or `KEY:`): the
	// value comes from the shell running Compose.
	PassThrough bool
}

// EnvFile is an `env_file:` reference.
type EnvFile struct {
	// Written is the path as written in the file.
	Written string
	// Path is resolved against Dir, relative to the root and slash-separated.
	// Empty if it points outside the root or is absolute.
	Path     string
	Required bool
	Line     int
	Column   int
	// Exists is filled in by the caller (project.Build), which does the I/O.
	Exists bool
}

// Interpolation is one variable substitution.
type Interpolation struct {
	Variable     string
	Line, Column int
	// HasDefault: ${VAR:-d}, ${VAR-d}, ${VAR:+a}, ${VAR+a}.
	HasDefault bool
	// Required: ${VAR:?err}, ${VAR?err} — Compose aborts if unset.
	Required bool
}

// interpRE matches $$ (escape), braced and bare substitutions. Nested
// substitutions inside a default are not reported (they only matter when
// the outer variable is unset).
var interpRE = regexp.MustCompile(`\$\$|\$\{(?P<bname>[A-Za-z_][A-Za-z0-9_]*)(?P<op>:?[-?+])?[^}]*\}|\$(?P<name>[A-Za-z_][A-Za-z0-9_]*)`)

// Parse parses a Compose file. It never fails: syntax errors are returned in
// File.Problem.
func Parse(data []byte, rel string) *File {
	f := &File{Path: rel, Dir: path.Dir(rel)}
	root, prob := yamlutil.Parse(data)
	if prob != nil {
		f.Problem = prob
		return f
	}
	if root == nil {
		return f
	}
	loc := yamlutil.NewLocator(data)
	f.Interpolations = interpolations(root, loc)

	services := yamlutil.Get(root, "services")
	for _, p := range yamlutil.Pairs(services) {
		svc := Service{Name: p[0].Value, Line: p[0].Line, Column: p[0].Column}
		body := yamlutil.Resolve(p[1])
		svc.Environment = environment(yamlutil.Get(body, "environment"))
		svc.EnvFiles = envFiles(yamlutil.Get(body, "env_file"), f.Dir)
		f.Services = append(f.Services, svc)
	}
	sort.SliceStable(f.Services, func(i, j int) bool { return f.Services[i].Name < f.Services[j].Name })
	return f
}

func interpolations(root *yaml.Node, loc *yamlutil.Locator) []Interpolation {
	var out []Interpolation
	bi, oi, ni := interpRE.SubexpIndex("bname"), interpRE.SubexpIndex("op"), interpRE.SubexpIndex("name")
	yamlutil.WalkScalars(root, func(s *yaml.Node, _ string, _ bool) {
		if !strings.Contains(s.Value, "$") {
			return
		}
		cur := loc.Cursor(s)
		for _, m := range interpRE.FindAllStringSubmatchIndex(s.Value, -1) {
			token := s.Value[m[0]:m[1]]
			if token == "$$" {
				cur.Find(token)
				continue
			}
			it := Interpolation{}
			if m[2*bi] >= 0 {
				it.Variable = s.Value[m[2*bi]:m[2*bi+1]]
				if m[2*oi] >= 0 {
					op := strings.TrimPrefix(s.Value[m[2*oi]:m[2*oi+1]], ":")
					it.HasDefault = op == "-" || op == "+"
					it.Required = op == "?"
				}
			} else {
				it.Variable = s.Value[m[2*ni]:m[2*ni+1]]
			}
			line, col := cur.Find(token)
			it.Line, it.Column = line, col+1 // point at the name, after '$'
			if strings.HasPrefix(token, "${") {
				it.Column++
			}
			out = append(out, it)
		}
	})
	return out
}

func environment(n *yaml.Node) []EnvVar {
	if n == nil {
		return nil
	}
	var out []EnvVar
	switch n.Kind {
	case yaml.MappingNode:
		for _, p := range yamlutil.Pairs(n) {
			v := yamlutil.Resolve(p[1])
			pass := v == nil || (v.Kind == yaml.ScalarNode && v.ShortTag() == "!!null")
			out = append(out, EnvVar{Name: p[0].Value, Line: p[0].Line, Column: p[0].Column, PassThrough: pass})
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			item = yamlutil.Resolve(item)
			if item == nil || item.Kind != yaml.ScalarNode || item.Value == "" {
				continue
			}
			name, _, hasValue := strings.Cut(item.Value, "=")
			out = append(out, EnvVar{Name: strings.TrimSpace(name), Line: item.Line, Column: item.Column, PassThrough: !hasValue})
		}
	}
	return out
}

func envFiles(n *yaml.Node, dir string) []EnvFile {
	if n == nil {
		return nil
	}
	items := []*yaml.Node{n}
	if n.Kind == yaml.SequenceNode {
		items = n.Content
	}
	var out []EnvFile
	for _, item := range items {
		item = yamlutil.Resolve(item)
		if item == nil {
			continue
		}
		ef := EnvFile{Required: true, Line: item.Line, Column: item.Column}
		switch item.Kind {
		case yaml.ScalarNode:
			ef.Written = item.Value
		case yaml.MappingNode:
			p := yamlutil.Get(item, "path")
			if p == nil {
				continue
			}
			ef.Written = p.Value
			if r := yamlutil.Get(item, "required"); r != nil && r.ShortTag() == "!!bool" && r.Value == "false" {
				ef.Required = false
			}
		default:
			continue
		}
		if ef.Written == "" {
			continue
		}
		ef.Path = resolve(dir, ef.Written)
		out = append(out, ef)
	}
	return out
}

// resolve joins a path written in the Compose file with its directory.
// Absolute paths, paths with interpolation and paths leaving the root
// resolve to "".
func resolve(dir, written string) string {
	w := strings.ReplaceAll(written, "\\", "/")
	if path.IsAbs(w) || strings.Contains(w, "$") || (len(w) > 1 && w[1] == ':') {
		return ""
	}
	p := joinRel(dir, w)
	if p == ".." || strings.HasPrefix(p, "../") {
		return ""
	}
	return p
}

func joinRel(dir, name string) string {
	if dir == "." || dir == "" {
		return path.Clean(name)
	}
	return path.Clean(dir + "/" + name)
}
