package web

import (
	"net"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

// ListenLoopback normalizes the host before binding and returns the same host
// for runtime URLs. Resolve localhost explicitly to avoid a DNS/bind mismatch.
func ListenLoopback(host string, port int) (net.Listener, string, error) {
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	if !isLoopbackHost(host) {
		return nil, "", apperr.New(apperr.CodePermissionDenied, "web host must be loopback")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, "", err
	}
	if err := validateLoopbackListener(listener); err != nil {
		_ = listener.Close()
		return nil, "", err
	}
	return listener, host, nil
}

func validateLoopbackListener(listener net.Listener) error {
	if listener != nil {
		if addr, ok := listener.Addr().(*net.TCPAddr); ok && addr.IP.IsLoopback() {
			return nil
		}
	}
	return apperr.New(apperr.CodePermissionDenied, "web listener must bind a loopback address")
}
