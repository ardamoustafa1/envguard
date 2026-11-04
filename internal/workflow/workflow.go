package workflow

import (
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ardamoustafa1/envguard/internal/yamlutil"
)

// Contexts of a Ref.
const (
	ContextSecrets = "secrets"
	ContextVars    = "vars"
	ContextEnv     = "env"
)

// Levels of an env: definition.
const (
	LevelWorkflow = "workflow"
	LevelJob      = "job"
	LevelStep     = "step"
)

// File is a parsed GitHub Actions workflow. Values are never stored.
type File struct {
	// Path is relative to the root, slash-separated.
	Path string
	Refs []Ref
	// Problem is set when the file is not valid YAML.
	Problem *yamlutil.Problem
}

// Ref is a secret/var/env reference in an expression, or an env: key.
type Ref struct {
	Name string
	// Context is "secrets", "vars" or "env".
	Context string
	// Define is true for keys under env: (workflow, job or step level);
	// false for ${{ secrets.X }}, ${{ vars.X }} and ${{ env.X }} references.
	Define bool
	// Level is set for definitions: "workflow", "job" or "step".
	Level        string
	Job          string
	Line, Column int
}

var (
	exprRE = regexp.MustCompile(`\$\{\{(.*?)\}\}`)
	// ctxRE finds secrets.X, vars.X, env.X and secrets['X'] / secrets["X"].
	ctxRE = regexp.MustCompile(`\b(secrets|vars|env)(?:\.([A-Za-z_][A-Za-z0-9_-]*)|\[\s*['"]([A-Za-z_][A-Za-z0-9_-]*)['"]\s*\])`)
)

// Parse parses a workflow file. It never fails: syntax errors are returned
// in File.Problem. secrets.GITHUB_TOKEN is ignored (GitHub provides it).
func Parse(data []byte, rel string) *File {
	f := &File{Path: rel}
	root, prob := yamlutil.Parse(data)
	if prob != nil {
		f.Problem = prob
		return f
	}
	if root == nil {
		return f
	}
	f.Refs = append(f.Refs, defines(yamlutil.Get(root, "env"), LevelWorkflow, "")...)
	for _, job := range yamlutil.Pairs(yamlutil.Get(root, "jobs")) {
		body := yamlutil.Resolve(job[1])
		f.Refs = append(f.Refs, defines(yamlutil.Get(body, "env"), LevelJob, job[0].Value)...)
		steps := yamlutil.Get(body, "steps")
		if steps == nil || steps.Kind != yaml.SequenceNode {
			continue
		}
		for _, step := range steps.Content {
			f.Refs = append(f.Refs, defines(yamlutil.Get(step, "env"), LevelStep, job[0].Value)...)
		}
	}
	f.Refs = append(f.Refs, references(root, yamlutil.NewLocator(data))...)
	return f
}

func defines(env *yaml.Node, level, job string) []Ref {
	var out []Ref
	for _, p := range yamlutil.Pairs(env) {
		if p[0].Value == "" {
			continue
		}
		out = append(out, Ref{Name: p[0].Value, Context: ContextEnv, Define: true, Level: level, Job: job,
			Line: p[0].Line, Column: p[0].Column})
	}
	return out
}

// references collects context accesses inside ${{ ... }} and in `if:`
// values, which are expressions even without ${{ }}.
func references(root *yaml.Node, loc *yamlutil.Locator) []Ref {
	var out []Ref
	yamlutil.WalkScalars(root, func(s *yaml.Node, parentKey string, isKey bool) {
		if isKey {
			return
		}
		var exprs []string
		if parentKey == "if" && !strings.Contains(s.Value, "${{") {
			exprs = []string{s.Value}
		} else {
			for _, m := range exprRE.FindAllStringSubmatch(s.Value, -1) {
				exprs = append(exprs, m[1])
			}
		}
		if len(exprs) == 0 {
			return
		}
		cur := loc.Cursor(s)
		for _, e := range exprs {
			for _, ix := range ctxRE.FindAllStringSubmatchIndex(e, -1) {
				token := e[ix[0]:ix[1]]
				if ix[0] > 0 && e[ix[0]-1] == '.' {
					continue // a property such as steps.x.outputs.env, not a context
				}
				ctx := e[ix[2]:ix[3]]
				var name string
				if ix[4] >= 0 {
					name = e[ix[4]:ix[5]]
				} else {
					name = e[ix[6]:ix[7]]
				}
				line, col := cur.Find(token)
				if ctx == ContextSecrets && name == "GITHUB_TOKEN" {
					continue
				}
				out = append(out, Ref{Name: name, Context: ctx, Line: line, Column: col})
			}
		}
	})
	return out
}
