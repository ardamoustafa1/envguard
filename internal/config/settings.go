package config

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/ardamoustafa1/envguard/internal/discovery"
)

// Flags carries command-line values. A nil pointer (or nil slice) means the
// flag was not given, so the config file or default applies.
type Flags struct {
	Root    string // --root; "" means "."
	Config  string // --config; "" means search the root
	NoColor *bool  // --no-color (the CLI also sets it from NO_COLOR)
	Verbose *bool  // --verbose
	FailOn  *string
	Include []string
	Exclude []string
}

// Settings is the effective, immutable configuration: defaults, overridden by
// the config file, overridden by flags. Treat all fields as read-only.
type Settings struct {
	// Root is the absolute root directory.
	Root string
	// ConfigPath is the config file used, or "" when none was found.
	ConfigPath string

	NoColor bool
	Verbose bool
	FailOn  string

	Include      []string
	Exclude      []string
	Environments map[string]Environment
	Rules        map[string]string
	Ignore       []IgnoreEntry
	Scanner      Scanner
	Profiles     []string
	Variables    map[string]Variable
	// VariablePositions locates each variable's key in ConfigPath.
	VariablePositions map[string]Position
}

// Defaults returns the settings used when there is no config file and no
// flags.
func Defaults() Settings {
	return Settings{Root: ".", FailOn: SeverityError}
}

// Load resolves the effective settings: it finds and strictly loads the
// config file (if any) and applies flags on top. Errors match ErrInvalid
// unless the root itself is unusable.
func Load(flags Flags) (*Settings, error) {
	root := flags.Root
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("--root: cannot read %q (%s); pass the directory of the repository to check, or run envguard from it",
			root, errText(err))
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("--root: %q is a file, not a directory; pass the directory of the repository to check", root)
	}

	path, err := Find(root, flags.Config)
	if err != nil {
		return nil, err
	}
	var file *File
	if path != "" {
		if file, err = LoadFile(path); err != nil {
			return nil, err
		}
	}
	s := Merge(Defaults(), file, flags)
	s.Root = abs
	if path != "" {
		s.ConfigPath = displayPath(path)
	}
	return &s, nil
}

// Merge applies a config file (may be nil) and then flags on top of base.
// Precedence: flags > config file > base. Slices and maps are copied so the
// result shares no memory with its inputs.
func Merge(base Settings, file *File, flags Flags) Settings {
	s := base
	if file != nil {
		if file.FailOn != "" {
			s.FailOn = file.FailOn
		}
		if file.Include != nil {
			s.Include = file.Include
		}
		if file.Exclude != nil {
			s.Exclude = file.Exclude
		}
		s.Environments = file.Environments
		s.Rules = file.Rules
		s.Ignore = file.Ignore
		s.Scanner = file.Scanner
		s.Profiles = file.Profiles
		s.Variables = file.Variables
		s.VariablePositions = file.Positions
	}
	if flags.Root != "" {
		s.Root = flags.Root
	}
	if flags.NoColor != nil {
		s.NoColor = *flags.NoColor
	}
	if flags.Verbose != nil {
		s.Verbose = *flags.Verbose
	}
	if flags.FailOn != nil {
		s.FailOn = *flags.FailOn
	}
	if flags.Include != nil {
		s.Include = flags.Include
	}
	if flags.Exclude != nil {
		s.Exclude = flags.Exclude
	}

	s.Include = slices.Clone(s.Include)
	s.Exclude = slices.Clone(s.Exclude)
	s.Environments = maps.Clone(s.Environments)
	s.Rules = maps.Clone(s.Rules)
	s.Ignore = slices.Clone(s.Ignore)
	s.Scanner.Languages = slices.Clone(s.Scanner.Languages)
	s.Scanner.IgnoreVariables = slices.Clone(s.Scanner.IgnoreVariables)
	s.Profiles = slices.Clone(s.Profiles)
	s.Variables = maps.Clone(s.Variables)
	s.VariablePositions = maps.Clone(s.VariablePositions)
	return s
}

// ValidateFailOn checks a --fail-on value.
func ValidateFailOn(v string) error {
	if !slices.Contains(enumValues("fail-on"), v) {
		return &usageError{fmt.Sprintf("--fail-on: invalid value; allowed: error, warning, info, never%s",
			suggestion(v, enumValues("fail-on")))}
	}
	return nil
}

// DiscoveryOptions converts settings into options for discovery.Discover.
func (s *Settings) DiscoveryOptions() discovery.Options {
	opts := discovery.Options{
		Root:    s.Root,
		Include: slices.Clone(s.Include),
		Exclude: slices.Clone(s.Exclude),
	}
	for _, name := range sortedKeys(s.Environments) {
		env := s.Environments[name]
		opts.Environments = append(opts.Environments, discovery.EnvironmentPatterns{
			Name:      name,
			Files:     slices.Clone(env.Files),
			Templates: slices.Clone(env.Templates),
		})
	}
	for _, l := range s.Scanner.Languages {
		opts.Languages = append(opts.Languages, discovery.Language(l))
	}
	return opts
}

// usageError is a bad command-line value. It matches ErrInvalid (so it exits
// with code 2 like every configuration error) without repeating that text.
type usageError struct{ msg string }

func (e *usageError) Error() string        { return e.msg }
func (e *usageError) Is(target error) bool { return target == ErrInvalid }
