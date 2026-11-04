package main

import (
	"runtime/debug"
	"strings"
)

// resolveBuildInfo fills in what -ldflags did not set. Release builds
// (GoReleaser, make build) inject version, commit and date. A plain
// `go install …@v1.2.3` does not, so the module version and the VCS stamp
// from debug.ReadBuildInfo are used instead.
func resolveBuildInfo(info buildInfo, read func() (*debug.BuildInfo, bool)) buildInfo {
	bi, ok := read()
	if !ok || bi == nil {
		return info
	}
	if info.Version == "dev" || info.Version == "" {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			info.Version = strings.TrimPrefix(v, "v")
		}
	}
	modified := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "none" || info.Commit == "" {
				info.Commit = s.Value[:min(len(s.Value), 12)]
			}
		case "vcs.time":
			if info.Date == "unknown" || info.Date == "" {
				info.Date = s.Value
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if modified && info.Commit != "none" && !strings.HasSuffix(info.Commit, "-dirty") {
		info.Commit += "-dirty"
	}
	return info
}
