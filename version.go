package main

import (
	"runtime/debug"
	"strings"
)

// VersionCore is the SemVer version core of portfwd.
// Meant to be be overridden at build time, but kept
// up-to-date sometimes to best support `go install`.
var VersionCore = "0.1.0"

// SemVer returns the semantic version of portfwd as
// built from VersionCore and debug build info.
func SemVer() string {
	semver := VersionCore

	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		var (
			revision string
			modified bool
		)
		for _, setting := range buildInfo.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}

		if revision != "" {
			i := len(revision)
			if i > 7 {
				i = 7
			}

			if !strings.Contains(semver, revision[:i]) {
				semver += "+" + revision[:i]
			}
		}

		if modified {
			semver += "*"
		}
	}

	return semver
}
