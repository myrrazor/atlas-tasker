package cli

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/buildinfo"
)

func TestVersionCommandText(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = oldVersion, oldCommit, oldBuildDate
	})
	buildinfo.Version = "v9.9.9-test"
	buildinfo.Commit = "abc123"
	buildinfo.BuildDate = "2026-05-07T04:00:00Z"

	out, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version failed: %v", err)
	}
	for _, want := range []string{
		"tracker v9.9.9-test",
		"commit: abc123",
		"build date: 2026-05-07T04:00:00Z",
		"go: " + runtime.Version(),
		"platform: " + runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("version output missing %q:\n%s", want, out)
		}
	}
}

func TestVersionCommandJSONContract(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = oldVersion, oldCommit, oldBuildDate
	})
	buildinfo.Version = "v9.9.9-test"
	buildinfo.Commit = "abc123"
	buildinfo.BuildDate = "2026-05-07T04:00:00Z"

	out, err := runCLI(t, "version", "--json")
	if err != nil {
		t.Fatalf("version --json failed: %v", err)
	}
	var got struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
		BuildDate     string `json:"build_date"`
		GoVersion     string `json:"go_version"`
		Platform      string `json:"platform"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse version json: %v\nraw=%s", err, out)
	}
	if got.FormatVersion != jsonFormatVersion || got.Kind != "tracker_version" || got.Version != "v9.9.9-test" || got.Commit != "abc123" || got.BuildDate != "2026-05-07T04:00:00Z" {
		t.Fatalf("unexpected version json: %+v", got)
	}
	if got.GoVersion != runtime.Version() || got.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("unexpected runtime metadata: %+v", got)
	}
}
