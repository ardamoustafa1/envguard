// Package yamlutil holds small helpers shared by the Compose and workflow
// parsers: parsing into a node tree with positions, walking scalars without
// following aliases (so alias bombs stay linear), merge-key aware mapping
// access, and locating tokens inside scalars in the raw source.
package yamlutil

import (
	"bytes"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Problem is a YAML syntax problem. Message never contains file content.
type Problem struct {
	Line, Column int
	Message      string
}

var (
	lineRE  = regexp.MustCompile(`line (\d+)`)
	quoteRE = regexp.MustCompile("`[^`]*`|\"[^\"]*\"|'[^']*'")
)

// Parse parses data and returns the root content node (nil for an empty
// document). On a syntax error it returns a Problem with a sanitized message.
func Parse(data []byte) (*yaml.Node, *Problem) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		msg := strings.TrimPrefix(err.Error(), "yaml: ")
		p := &Problem{Message: "invalid YAML: " + quoteRE.ReplaceAllString(msg, "value")}
		if m := lineRE.FindStringSubmatch(msg); m != nil {
			for _, c := range m[1] {
				p.Line = p.Line*10 + int(c-'0')
			}
			p.Column = 1
		}
		return nil, p
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	return doc.Content[0], nil
}

// Resolve follows an alias node one level.
func Resolve(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		return n.Alias
	}
	return n
}

// Pairs returns the key/value pairs of a mapping, expanding `<<` merge keys
// (one level of aliases, so expansion stays bounded). Explicit keys win over
// merged ones.
func Pairs(m *yaml.Node) [][2]*yaml.Node {
	m = Resolve(m)
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	var own, merged [][2]*yaml.Node
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Value == "<<" && k.Kind == yaml.ScalarNode {
			targets := []*yaml.Node{Resolve(v)}
			if t := Resolve(v); t != nil && t.Kind == yaml.SequenceNode {
				targets = t.Content
			}
			for _, t := range targets {
				t = Resolve(t)
				if t == nil || t.Kind != yaml.MappingNode {
					continue
				}
				for j := 0; j+1 < len(t.Content); j += 2 {
					merged = append(merged, [2]*yaml.Node{t.Content[j], t.Content[j+1]})
				}
			}
			continue
		}
		if k.Kind != yaml.ScalarNode {
			// `{a: b}: c` or `*alias: c`: a key that is not a name means
			// nothing to any consumer (service, variable, job, ...).
			continue
		}
		own = append(own, [2]*yaml.Node{k, v})
	}
	seen := map[string]bool{}
	for _, p := range own {
		seen[p[0].Value] = true
	}
	for _, p := range merged {
		if !seen[p[0].Value] {
			own = append(own, p)
			seen[p[0].Value] = true
		}
	}
	return own
}

// Get returns the value for key in a mapping (merge keys included), or nil.
func Get(m *yaml.Node, key string) *yaml.Node {
	for _, p := range Pairs(m) {
		if p[0].Value == key {
			return Resolve(p[1])
		}
	}
	return nil
}

// WalkScalars calls fn for every scalar node under n (keys included), never
// following aliases: each scalar is visited exactly once, at its definition.
// parentKey is the mapping key whose value contains the scalar ("" if none).
func WalkScalars(n *yaml.Node, fn func(s *yaml.Node, parentKey string, isKey bool)) {
	walk(n, "", fn)
}

func walk(n *yaml.Node, parentKey string, fn func(*yaml.Node, string, bool)) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.ScalarNode:
		fn(n, parentKey, false)
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind == yaml.ScalarNode {
				fn(k, parentKey, true)
			}
			walk(v, k.Value, fn)
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, c := range n.Content {
			walk(c, parentKey, fn)
		}
	}
}

// Locator maps tokens found in decoded scalar values back to positions in
// the raw source.
type Locator struct {
	src    []byte
	starts []int
}

// NewLocator indexes src.
func NewLocator(src []byte) *Locator {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &Locator{src: src, starts: starts}
}

// Cursor finds successive tokens within one scalar node.
type Cursor struct {
	l   *Locator
	n   *yaml.Node
	off int // byte offset where the next search starts
}

// Cursor starts searching at the node's position.
func (l *Locator) Cursor(n *yaml.Node) *Cursor {
	return &Cursor{l: l, n: n, off: l.offset(n.Line, n.Column)}
}

// searchWindow bounds how far past the node start a token is searched.
const searchWindow = 1 << 16

// Find returns the 1-based line and character column of the next occurrence
// of token. If the raw text differs from the decoded value (escapes), it
// falls back to the node position.
func (c *Cursor) Find(token string) (line, col int) {
	src := c.l.src
	end := min(len(src), c.off+searchWindow)
	if c.off <= end {
		if k := bytes.Index(src[c.off:end], []byte(token)); k >= 0 {
			pos := c.off + k
			c.off = pos + len(token)
			return c.l.position(pos)
		}
	}
	return c.n.Line, c.n.Column
}

func (l *Locator) offset(line, col int) int {
	if line < 1 || line > len(l.starts) {
		return len(l.src)
	}
	off := l.starts[line-1]
	for i := 1; i < col && off < len(l.src) && l.src[off] != '\n'; i++ {
		_, size := utf8.DecodeRune(l.src[off:])
		off += size
	}
	return off
}

func (l *Locator) position(off int) (line, col int) {
	lo, hi := 0, len(l.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if l.starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1, 1 + utf8.RuneCount(l.src[l.starts[lo]:off])
}
