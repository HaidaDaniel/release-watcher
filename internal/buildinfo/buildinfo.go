package buildinfo

import (
	"runtime"
	"time"
)

const Name = "release-watcher"

// These variables are overridden with -ldflags by scripts/build.sh.
var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	builtAt   = "unknown"
	buildMode = "development"
)

// Info is the stable build identity exposed by the version command.
type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuiltAt   string `json:"built_at"`
	BuildMode string `json:"build_mode"`
	GoVersion string `json:"go_version"`
}

func Get() Info {
	return Info{
		Name:      Name,
		Version:   version,
		Commit:    commit,
		BuiltAt:   builtAt,
		BuildMode: buildMode,
		GoVersion: runtime.Version(),
	}
}

// ReleaseReady reports whether the build carries explicit release identity.
// A development build is honest but is not evidence of a deployed release.
func ReleaseReady(i Info) bool {
	if i.BuildMode != "release" {
		return false
	}
	if len(i.Commit) != 40 {
		return false
	}
	if _, err := time.Parse(time.RFC3339, i.BuiltAt); err != nil {
		return false
	}
	return true
}
