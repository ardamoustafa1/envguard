// Package compose parses Docker Compose files: per-service environment
// (map and list forms), env_file references (string, list and long syntax),
// and every ${VAR} interpolation with its position. Interpolation reads the
// .env file next to the Compose file; see File.AdjacentEnv.
package compose
