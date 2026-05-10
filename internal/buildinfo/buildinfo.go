package buildinfo

import "runtime"

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
func Current() Info {
	return Info{
		Version:   cleanDefault(Version, "dev"),
		Commit:    cleanDefault(Commit, "unknown"),
		BuildDate: cleanDefault(BuildDate, "unknown"),
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}

func cleanDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
