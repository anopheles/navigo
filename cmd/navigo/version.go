package main

import (
	"fmt"
	"runtime/debug"
)

// Set by the release build's ldflags (.goreleaser.yaml). A go install build
// leaves them empty and versionString falls back to the module version.
var version, commit, date string

// versionString is what navigo --version prints, e.g.
// "navigo v0.1.0 (abc1234, 2026-10-20T12:00:00Z)".
func versionString() string {
	v, c, d := version, commit, date
	if info, ok := debug.ReadBuildInfo(); ok && v == "" {
		v = info.Main.Version
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				c = s.Value[:min(7, len(s.Value))]
			case "vcs.time":
				d = s.Value
			}
		}
	}
	if v == "" {
		v = "(devel)"
	}
	if c == "" {
		return "navigo " + v
	}
	return fmt.Sprintf("navigo %s (%s, %s)", v, c, d)
}
