package buildinfo

import (
	"runtime"
	"runtime/debug"
)

var (
	// Version is stamped by release builds. Local source builds keep the dev default.
	Version = "dev"
	// Commit is the source commit stamped by release builds.
	Commit = "unknown"
	// BuildDate is an RFC3339 UTC timestamp stamped by release builds.
	BuildDate = "unknown"
)

// Info is the stable public shape behind `tracker version --json`.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Current returns release metadata plus runtime details for the running binary.
// Unstamped builds fall back to VCS settings from the Go build info so a local
// binary still shows a commit instead of a blank version.
func Current() Info {
	version := cleanDefault(Version, "dev")
	commit := cleanDefault(Commit, "unknown")
	date := cleanDefault(BuildDate, "unknown")
	if info, ok := debug.ReadBuildInfo(); ok {
		var revision, vcsTime, modified string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.time":
				vcsTime = setting.Value
			case "vcs.modified":
				modified = setting.Value
			}
		}
		if commit == "unknown" && revision != "" {
			commit = shortRev(revision)
			if modified == "true" {
				commit += "-dirty"
			}
		}
		if date == "unknown" && vcsTime != "" {
			date = vcsTime
		}
		if version == "dev" {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				version = info.Main.Version
			} else if revision != "" {
				version = "dev+" + shortRev(revision)
			}
		}
	}
	return Info{
		Version:   version,
		Commit:    commit,
		BuildDate: date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}

func shortRev(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

func cleanDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
