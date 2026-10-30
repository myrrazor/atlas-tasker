package web

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSessionTokenDoesNotRewriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-session.json")
	session, err := OpenSession(path)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	token, err := ReadSessionToken(path)
	if err != nil || token != session.Token() {
		t.Fatalf("ReadSessionToken = %q, %v; want current token", token, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("ReadSessionToken must not rewrite the session file")
	}
	if _, err := ReadSessionToken(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing session file should fail")
	}
}

func TestSessionURLForMatchesServeSessionURLShape(t *testing.T) {
	token := strings.Repeat("ab", 32)
	got, err := SessionURLFor("http://127.0.0.1:17951/board", token)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if u.Path != "/board" || u.Query().Get("token") != token || u.Host != "127.0.0.1:17951" {
		t.Fatalf("unexpected session URL %q", got)
	}
	for _, bad := range []string{"http://example.com/board", "http://0.0.0.0:1/board", "https://10.0.0.1/board", "::"} {
		if _, err := SessionURLFor(bad, token); err == nil {
			t.Fatalf("SessionURLFor(%q) must refuse a non-loopback URL", bad)
		}
	}
}
