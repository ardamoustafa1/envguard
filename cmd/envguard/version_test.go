package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveBuildInfo(t *testing.T) {
	stamped := func(version string, settings ...debug.BuildSetting) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: version}, Settings: settings}, true
		}
	}
	dev := buildInfo{Version: "dev", Commit: "none", Date: "unknown"}
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef0123"}
	when := debug.BuildSetting{Key: "vcs.time", Value: "2026-11-04T10:00:00Z"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	tests := []struct {
		name string
		in   buildInfo
		read func() (*debug.BuildInfo, bool)
		want buildInfo
	}{
		{"go install @version", dev, stamped("v1.4.2"), buildInfo{"1.4.2", "none", "unknown"}},
		{"local build with vcs", dev, stamped("(devel)", rev, when), buildInfo{"dev", "0123456789ab", "2026-11-04T10:00:00Z"}},
		{"dirty tree", dev, stamped("(devel)", rev, dirty), buildInfo{"dev", "0123456789ab-dirty", "unknown"}},
		{"ldflags win", buildInfo{"1.0.0", "abc", "2026-01-01"}, stamped("v9.9.9", rev, when), buildInfo{"1.0.0", "abc", "2026-01-01"}},
		{"no build info", dev, func() (*debug.BuildInfo, bool) { return nil, false }, dev},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveBuildInfo(tt.in, tt.read); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
