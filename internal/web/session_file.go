package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateWebSession keeps the loopback session cookie and CSRF secret
// stable across process restarts. The secrets stay on local disk (mode 0600)
// and are never put in a URL. A matching cookie from an already-open tab keeps
// working; there is no unauthenticated reclaim path.
func LoadOrCreateWebSession(path string) (string, string, error) {
	if raw, err := os.ReadFile(path); err == nil {
		var doc struct {
			Token string `json:"token"`
			CSRF  string `json:"csrf"`
		}
		if json.Unmarshal(raw, &doc) == nil && validSessionSecret(doc.Token) && validSessionSecret(doc.CSRF) {
			return doc.Token, doc.CSRF, nil
		}
	}
	token := randomToken()
	csrf := randomToken()
	if err := writeSessionFile(path, token, csrf); err != nil {
		return "", "", err
	}
	return token, csrf, nil
}

func writeSessionFile(path, token, csrf string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]string{"token": token, "csrf": csrf})
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func validSessionSecret(value string) bool {
	if len(value) < 32 || len(value) > 128 {
		return false
	}
	return strings.Trim(value, "0123456789abcdefABCDEF") == ""
}
