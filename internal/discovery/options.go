package discovery

// EnvironmentPatterns maps glob patterns to an environment name. Matching
// files are classified as env files (Files) or templates (Templates) of that
// environment, overriding name-based inference.
type EnvironmentPatterns struct {
	Name      string
	Files     []string
	Templates []string
}

// Options configures Discover. Zero values select defaults.
type Options struct {
	// Root is the directory to walk.
	Root string
	// Include, when non-empty, restricts discovered files to those matching
	// at least one glob. Exclude skips matching files and directories.
	// Patterns without a '/' match base names at any depth.
	Include []string
	Exclude []string
	// Environments are explicit environment mappings, checked in order.
	Environments []EnvironmentPatterns
	// Languages limits source discovery; empty means all languages.
	Languages []Language

	MaxDepth      int   // default DefaultMaxDepth
	MaxFiles      int   // default DefaultMaxFiles
	MaxSourceSize int64 // default DefaultMaxSourceSize
}
