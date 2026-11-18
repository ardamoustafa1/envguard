package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config file names searched in the root, in order.
var fileNames = [...]string{".envguard.yaml", ".envguard.yml"}

// maxConfigSize caps how much of a config file is read.
const maxConfigSize = 1 << 20

// ErrInvalid is matched (errors.Is) by every configuration error: a missing
// explicit config file, unreadable or malformed YAML, unknown keys or invalid
// values. The CLI maps it to exit code 2.
var ErrInvalid = errors.New("invalid configuration")

// Problem is one configuration problem with its position.
type Problem struct {
	File    string
	Line    int
	Column  int
	Message string
}

func (p Problem) String() string {
	if p.Line == 0 {
		return fmt.Sprintf("%s: %s", p.File, p.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s", p.File, p.Line, p.Column, p.Message)
}

// Error is returned for an invalid configuration file. It lists every
// problem found, in file order. Messages never contain configuration values
// other than key names.
type Error struct {
	File     string
	Problems []Problem
}

func (e *Error) Error() string {
	var b strings.Builder
	if len(e.Problems) == 1 {
		return "invalid configuration: " + e.Problems[0].String()
	}
	fmt.Fprintf(&b, "invalid configuration: %d problems in %s", len(e.Problems), e.File)
	for _, p := range e.Problems {
		b.WriteString("\n  ")
		b.WriteString(p.String())
	}
	return b.String()
}

// Is makes errors.Is(err, ErrInvalid) true.
func (e *Error) Is(target error) bool { return target == ErrInvalid }

// Find returns the config file to use. explicit is the --config value ("" if
// unset) and must exist when given. Otherwise root is searched for
// .envguard.yaml and .envguard.yml; "" means no config file (which is fine).
func Find(root, explicit string) (string, error) {
	if explicit != "" {
		info, err := os.Stat(explicit)
		if err != nil {
			return "", &Error{File: displayPath(explicit), Problems: []Problem{{
				File: displayPath(explicit), Message: "config file not found or unreadable: " + errText(err),
			}}}
		}
		if info.IsDir() {
			return "", &Error{File: displayPath(explicit), Problems: []Problem{{
				File: displayPath(explicit), Message: "config path is a directory, expected a file",
			}}}
		}
		if !info.Mode().IsRegular() {
			return "", nonRegularConfig(explicit)
		}
		return explicit, nil
	}
	var found []string
	for _, name := range fileNames {
		p := filepath.Join(root, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			if !info.Mode().IsRegular() {
				return "", nonRegularConfig(p)
			}
			found = append(found, p)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	default:
		return "", &Error{File: displayPath(found[0]), Problems: []Problem{{
			File:    displayPath(found[0]),
			Message: "both .envguard.yaml and .envguard.yml exist; keep only one",
		}}}
	}
}

// LoadFile reads, strictly decodes and validates a config file.
func LoadFile(path string) (*File, error) {
	// Check before opening: opening a FIFO can wait forever for a writer.
	if info, err := os.Stat(path); err == nil && !info.Mode().IsRegular() {
		return nil, nonRegularConfig(path)
	}
	f, err := os.Open(path) // #nosec G304 -- reading the user's chosen config file is the point
	if err != nil {
		return nil, &Error{File: displayPath(path), Problems: []Problem{{File: displayPath(path), Message: "cannot read config file: " + errText(err)}}}
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxConfigSize+1))
	if err != nil {
		return nil, &Error{File: displayPath(path), Problems: []Problem{{File: displayPath(path), Message: "cannot read config file: " + errText(err)}}}
	}
	if len(data) > maxConfigSize {
		return nil, &Error{File: displayPath(path), Problems: []Problem{{File: displayPath(path), Message: "config file is larger than 1 MiB"}}}
	}
	return Parse(data, displayPath(path))
}

func nonRegularConfig(path string) *Error {
	name := displayPath(path)
	return &Error{File: name, Problems: []Problem{{File: name, Message: "config path is not a regular file"}}}
}

// Parse strictly decodes and validates config data. name is used in messages.
func Parse(data []byte, name string) (*File, error) {
	d := &decoder{file: name, nodes: map[string]*yaml.Node{}}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, d.fail(0, 0, "invalid YAML: "+sanitizeYAMLError(err))
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, d.fail(1, 1, `config file is empty; it must contain at least "version: 1"`)
	}
	root := doc.Content[0]
	d.walk(root, reflect.TypeFor[File](), "")
	if d.shapeErrors {
		// Values cannot be decoded reliably; report what we have.
		return nil, d.err()
	}

	// Unknown keys do not prevent decoding (they are simply not stored), so
	// value validation still runs and every problem is reported in one pass.
	var f File
	if err := root.Decode(&f); err != nil {
		if len(d.problems) > 0 {
			// The walk already explained what is wrong (e.g. a duplicate
			// key, which the YAML decoder rejects too); its problems are
			// located and specific, the decoder's text is not.
			return nil, d.err()
		}
		// The walk already checked shapes and scalar types, so this is rare.
		return nil, d.fail(root.Line, root.Column, "cannot decode configuration: "+sanitizeYAMLError(err))
	}
	d.validate(&f)
	if len(d.problems) > 0 {
		return nil, d.err()
	}
	for name := range f.Variables {
		if k := d.keys[joinPath("variables", name)]; k != nil {
			if f.Positions == nil {
				f.Positions = map[string]Position{}
			}
			f.Positions[name] = Position{Line: k.Line, Column: k.Column}
		}
	}
	return &f, nil
}

type decoder struct {
	file     string
	problems []Problem
	// shapeErrors is set when a node has the wrong kind or scalar type, which
	// makes decoding unreliable. Unknown keys do not set it.
	shapeErrors bool
	// nodes maps a key path ("rules.EG001", "ignore[0].rule") to its value
	// node; keys maps the same paths to the key node.
	nodes map[string]*yaml.Node
	keys  map[string]*yaml.Node
}

func (d *decoder) addf(n *yaml.Node, format string, args ...any) {
	line, col := 0, 0
	if n != nil {
		line, col = n.Line, n.Column
	}
	d.problems = append(d.problems, Problem{File: d.file, Line: line, Column: col, Message: fmt.Sprintf(format, args...)})
}

func (d *decoder) fail(line, col int, msg string) error {
	d.problems = append(d.problems, Problem{File: d.file, Line: line, Column: col, Message: msg})
	return d.err()
}

func (d *decoder) err() error {
	slices.SortStableFunc(d.problems, func(a, b Problem) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		return a.Column - b.Column
	})
	return &Error{File: d.file, Problems: d.problems}
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func label(path string) string {
	if path == "" {
		return "the top level"
	}
	return fmt.Sprintf("%q", path)
}

// walk checks node shapes against t and records unknown keys with
// suggestions. It never reports values, only key names and kinds.
func (d *decoder) walk(n *yaml.Node, t reflect.Type, path string) {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" {
		return
	}
	d.nodes[path] = n

	switch t.Kind() {
	case reflect.Pointer:
		d.walk(n, t.Elem(), path)
	case reflect.Struct:
		if !d.expect(n, yaml.MappingNode, path, "a mapping") {
			return
		}
		fields := yamlFields(t)
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		seen := map[string]*yaml.Node{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if d.duplicate(seen, k, path) {
				continue
			}
			f, ok := fields[k.Value]
			if !ok {
				d.addf(k, "unknown key %q in %s%s", k.Value, label(path), suggestion(k.Value, names))
				continue
			}
			p := joinPath(path, k.Value)
			d.keyNode(p, k)
			d.walk(v, f.Type, p)
		}
	case reflect.Map:
		if !d.expect(n, yaml.MappingNode, path, "a mapping") {
			return
		}
		seen := map[string]*yaml.Node{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind == yaml.ScalarNode && d.duplicate(seen, k, path) {
				continue
			}
			if k.Kind != yaml.ScalarNode {
				d.shapeErrors = true
				d.addf(k, "keys in %s must be plain strings", label(path))
				continue
			}
			p := joinPath(path, k.Value)
			d.keyNode(p, k)
			d.walk(v, t.Elem(), p)
		}
	case reflect.Slice:
		if !d.expect(n, yaml.SequenceNode, path, "a list") {
			return
		}
		for i, item := range n.Content {
			d.walk(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i))
		}
	case reflect.String:
		d.expect(n, yaml.ScalarNode, path, "a string")
	case reflect.Int:
		if d.expect(n, yaml.ScalarNode, path, "an integer") && n.ShortTag() != "!!int" {
			d.shapeErrors = true
			d.addf(n, "%s must be an integer", label(path))
		}
	case reflect.Float64:
		if d.expect(n, yaml.ScalarNode, path, "a number") && n.ShortTag() != "!!int" && n.ShortTag() != "!!float" {
			d.shapeErrors = true
			d.addf(n, "%s must be a number", label(path))
		}
	case reflect.Bool:
		if d.expect(n, yaml.ScalarNode, path, "a boolean") && n.ShortTag() != "!!bool" {
			d.shapeErrors = true
			d.addf(n, "%s must be true or false", label(path))
		}
	default:
		d.shapeErrors = true
		d.addf(n, "internal error: unsupported field type for %s", label(path))
	}
}

// duplicate reports k if its name was already used in the same mapping.
func (d *decoder) duplicate(seen map[string]*yaml.Node, k *yaml.Node, path string) bool {
	if first, ok := seen[k.Value]; ok {
		d.addf(k, "duplicate key %q in %s (first defined at line %d)", k.Value, label(path), first.Line)
		return true
	}
	seen[k.Value] = k
	return false
}

func (d *decoder) keyNode(path string, k *yaml.Node) {
	if d.keys == nil {
		d.keys = map[string]*yaml.Node{}
	}
	d.keys[path] = k
}

func (d *decoder) expect(n *yaml.Node, kind yaml.Kind, path, what string) bool {
	if n.Kind == kind {
		return true
	}
	d.shapeErrors = true
	d.addf(n, "%s must be %s, got %s", label(path), what, kindName(n.Kind))
	return false
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		return "a scalar value"
	default:
		return "an unsupported YAML node"
	}
}

// yamlFields maps yaml key names to struct fields.
func yamlFields(t reflect.Type) map[string]reflect.StructField {
	out := make(map[string]reflect.StructField, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = f
	}
	return out
}

// yamlValueRE matches the quoted value snippets yaml.v3 puts into type errors
// (e.g. "cannot unmarshal !!str `abc` into int"), which must not be echoed.
var yamlValueRE = regexp.MustCompile("`[^`]*`")

func sanitizeYAMLError(err error) string {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	// The decoder joins several problems with newlines; keep one line.
	msg = strings.Join(strings.Fields(msg), " ")
	return yamlValueRE.ReplaceAllString(msg, "value")
}

func errText(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

func displayPath(p string) string { return filepath.ToSlash(filepath.Clean(p)) }

type yamlNode = yaml.Node
