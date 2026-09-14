package slashcmd

import (
	"reflect"
	"testing"
)

func TestParseUninstall(t *testing.T) {
	got, err := Parse("/uninstall")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"uninstall"}) {
		t.Fatalf("got %#v", got)
	}
	got, err = Parse("/uninstall --yes")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"uninstall", "--yes"}) {
		t.Fatalf("got %#v", got)
	}
}
