package web

import (
	"context"
	"net"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func TestListenLoopbackNormalizesBeforeBinding(t *testing.T) {
	for _, host := range []string{"", "localhost", "127.0.0.1"} {
		t.Run(host, func(t *testing.T) {
			listener, normalized, err := ListenLoopback(host, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			if !listener.Addr().(*net.TCPAddr).IP.IsLoopback() || normalized != "127.0.0.1" {
				t.Fatalf("unsafe or mismatched bind: %s %s", listener.Addr(), normalized)
			}
		})
	}
	for _, host := range []string{"0.0.0.0", "::", "192.0.2.1"} {
		listener, _, err := ListenLoopback(host, 0)
		if listener != nil {
			listener.Close()
			t.Fatal("non-loopback listener was opened")
		}
		if apperr.CodeOf(err) != apperr.CodePermissionDenied {
			t.Fatalf("host %s: %v", host, err)
		}
	}
}

func TestLoopbackHostAllowsLocalhostAndLoopbackIP(t *testing.T) {
	if !loopbackHostAllowed("localhost:7432", "127.0.0.1", 7432) {
		t.Fatal("localhost should reach a 127.0.0.1 listener on the same port")
	}
	if !loopbackHostAllowed("127.0.0.1:7432", "localhost", 7432) {
		t.Fatal("127.0.0.1 should reach a localhost listener on the same port")
	}
	if loopbackHostAllowed("evil.example:7432", "127.0.0.1", 7432) {
		t.Fatal("foreign host must stay rejected")
	}
	if loopbackHostAllowed("localhost:7433", "127.0.0.1", 7432) {
		t.Fatal("a different loopback port must stay rejected")
	}
}

func TestServeRejectsWildcardListener(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &Server{}
	if err := server.Serve(context.Background(), listener); apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("wildcard listener accepted: %v", err)
	}
}
