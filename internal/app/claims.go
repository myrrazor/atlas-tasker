package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

type claimRecord struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *App) claimsDir() string {
	return filepath.Join(a.stateDir, "claims")
}

func (a *App) claimPath(token string) string {
	return filepath.Join(a.claimsDir(), token+".json")
}

// IssueClaim writes a short-lived single-use bootstrap token into private state
// so a caller process can open a URL served by a different Home process.
func (a *App) IssueClaim() (string, error) {
	token := randomID()
	rec := claimRecord{Token: token, ExpiresAt: a.now().Add(2 * time.Minute)}
	if err := os.MkdirAll(a.claimsDir(), 0o700); err != nil {
		return "", err
	}
	if err := atomicJSON(a.claimPath(token), rec); err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeClaim deletes and validates a claim file. Rename is the atomic
// single-use gate across processes. It never accepts the server cookie secret.
func (a *App) ConsumeClaim(token string) error {
	token = strings.TrimSpace(token)
	if token == "" || strings.Contains(token, "/") || strings.Contains(token, "..") {
		return apperr.New(apperr.CodePermissionDenied, "invalid session claim")
	}
	path := a.claimPath(token)
	taken := path + ".taken-" + randomID()
	if err := os.Rename(path, taken); err != nil {
		return apperr.New(apperr.CodePermissionDenied, "invalid session claim")
	}
	defer func() { _ = os.Remove(taken) }()
	raw, err := os.ReadFile(taken)
	if err != nil {
		return apperr.New(apperr.CodePermissionDenied, "invalid session claim")
	}
	var rec claimRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return apperr.New(apperr.CodePermissionDenied, "invalid session claim")
	}
	if rec.Token != token {
		return apperr.New(apperr.CodePermissionDenied, "invalid session claim")
	}
	if rec.ExpiresAt.IsZero() || a.now().After(rec.ExpiresAt) {
		return apperr.New(apperr.CodePermissionDenied, "session claim expired")
	}
	return nil
}

func (a *App) ClaimURL(base string) (string, error) {
	token, err := a.IssueClaim()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base, "/") + "/#claim=" + token, nil
}

func SessionClaimPath(token string) string {
	return "/session/claim/" + strings.TrimSpace(token)
}
