package service_test

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/setup"
)

func TestCanonicalUserStateDirMatchesSetupDefault(t *testing.T) {
	xdg := func(key string) string {
		if key == "XDG_STATE_HOME" {
			return "/var/state"
		}
		return ""
	}
	got, err := service.CanonicalUserStateDir("/tmp/home", xdg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := setup.DefaultStateDir("/tmp/home", xdg)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || got != "/var/state/atlas-tasker" {
		t.Fatalf("xdg: service %s setup %s", got, want)
	}

	none := func(string) string { return "" }
	got, err = service.CanonicalUserStateDir("/tmp/home", none)
	if err != nil {
		t.Fatal(err)
	}
	want, err = setup.DefaultStateDir("/tmp/home", none)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("machine default: service %s setup %s", got, want)
	}
}
