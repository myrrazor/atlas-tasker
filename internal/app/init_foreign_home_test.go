package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A different Atlas Home already listening on the configured port must not be
// reported as this machine's Home by `tracker init`.
func TestInitDoesNotClaimForeignAtlasHomeURL(t *testing.T) {
	const foreignID = "inst_foreign_test_home"
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Atlas-Service", "atlas-home")
		w.Header().Set("X-Atlas-Instance", foreignID)
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "atlas_home", "instance_id": foreignID})
	}))
	defer foreign.Close()
	host, portText, err := net.SplitHostPort(strings.TrimPrefix(foreign.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)

	a := testApp(t)
	spawner := &RecordingSpawner{}
	a.opts.Process = spawner
	a.opts.Probe = HTTPProber{}
	if _, err := a.UpdateSettings(context.Background(), MachineSettingsPatch{Service: &ServiceSettings{
		Bind: host, Port: port, Enabled: true, AutoStart: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if a.Settings().InstanceID == foreignID {
		t.Fatal("test needs distinct instance ids")
	}
	root := filepath.Join(a.Home(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := a.Init(context.Background(), InitOptions{Root: root, Register: true, DefaultProject: true})
	if err != nil {
		t.Fatalf("init should still create the workspace: %v", err)
	}
	if spawner.Calls != 0 {
		t.Fatalf("init spawned a Home onto an occupied port: %d", spawner.Calls)
	}
	foreignURL := homeURL(host, port)
	if result.Service == nil {
		t.Fatal("missing service status")
	}
	if result.Service.URL != "" || result.Service.Running || result.Service.IdentityOK {
		t.Fatalf("service status claims the foreign Home: %+v", result.Service)
	}
	if result.Service.PortOwner != PortOwnerOtherAtlas || result.Service.OwnerInstanceID != foreignID || result.Service.OwnerURL != foreignURL {
		t.Fatalf("service status should name the foreign owner: %+v", result.Service)
	}
	if strings.Contains(result.Summary, foreignURL) {
		t.Fatalf("summary prints the foreign URL as ours: %q", result.Summary)
	}
	for _, want := range []string{"another Atlas instance", foreignID, "not this one", "service.port", "stop that instance"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("summary missing %q: %q", want, result.Summary)
		}
	}
	found := false
	for _, step := range result.Steps {
		if step.Name == "service" {
			found = true
			if step.Status != InitStepConflict || strings.Contains(step.Detail, foreignURL) {
				t.Fatalf("service step: %+v", step)
			}
		}
	}
	if !found {
		t.Fatalf("missing service step: %+v", result.Steps)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `"url":""`) || !strings.Contains(string(raw), `"port_owner":"other_atlas_instance"`) {
		t.Fatalf("json not truthful: %s", raw)
	}
}
