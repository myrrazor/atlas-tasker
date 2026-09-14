package web

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

func loopbackHostAllowed(got string, wantHost string, wantPort int) bool {
	got = strings.TrimSpace(got)
	if got == "" {
		return false
	}
	host := got
	port := ""
	if h, p, err := net.SplitHostPort(got); err == nil {
		host, port = h, p
	} else if strings.HasPrefix(got, "[") {
		return false
	}
	host = strings.Trim(host, "[]")
	if !isLoopbackHost(host) {
		return false
	}
	if !strings.EqualFold(host, wantHost) {
		return false
	}
	if port == "" {
		return true
	}
	if wantPort <= 0 {
		return true
	}
	return port == strconv.Itoa(wantPort)
}

func exactOriginAllowed(raw string, wantHost string, wantPort int) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return loopbackHostAllowed(u.Host, wantHost, wantPort)
}
