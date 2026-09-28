package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// A local board session survives restarts, then dies. Rotation keeps an
	// already-open tab working by accepting the previous token while Set-Cookie
	// hands the tab the new one.
	sessionLifetime    = 14 * 24 * time.Hour
	sessionRotateEvery = 24 * time.Hour
	sessionOverlap     = 24 * time.Hour
)

// Session is the on-disk loopback session. CSRF stays stable across token
// rotation so a form that is already open still submits.
type Session struct {
	mu            sync.Mutex
	path          string
	token         string
	previous      string
	previousUntil time.Time
	csrf          string
	issued        time.Time
	expires       time.Time
}

type sessionDocument struct {
	Token         string    `json:"token"`
	PreviousToken string    `json:"previous_token,omitempty"`
	PreviousUntil time.Time `json:"previous_until,omitempty"`
	CSRF          string    `json:"csrf"`
	IssuedAt      time.Time `json:"issued_at,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
}

// LoadOrCreateWebSession keeps the loopback session cookie and CSRF secret
// stable across process restarts. The secrets stay on local disk (mode 0600)
// and are never put in a URL. A matching cookie from an already-open tab keeps
// working; there is no unauthenticated reclaim path.
func LoadOrCreateWebSession(path string) (string, string, error) {
	session, err := OpenSession(path)
	if err != nil {
		return "", "", err
	}
	return session.Token(), session.CSRF(), nil
}

// OpenSession loads or creates the session file and remembers it so later
// requests can rotate the token without dropping open tabs.
func OpenSession(path string) (*Session, error) {
	now := time.Now().UTC()
	if raw, err := os.ReadFile(path); err == nil {
		var doc sessionDocument
		if json.Unmarshal(raw, &doc) == nil && validSessionSecret(doc.Token) && validSessionSecret(doc.CSRF) {
			session := &Session{
				path:          path,
				token:         doc.Token,
				previous:      doc.PreviousToken,
				previousUntil: doc.PreviousUntil,
				csrf:          doc.CSRF,
				issued:        doc.IssuedAt.UTC(),
				expires:       doc.ExpiresAt.UTC(),
			}
			if session.issued.IsZero() || session.expires.IsZero() {
				session.issued = now
				session.expires = now.Add(sessionLifetime)
				if err := session.write(); err != nil {
					return nil, err
				}
			}
			if now.After(session.expires) {
				if err := session.hardRotate(now); err != nil {
					return nil, err
				}
			}
			return session, nil
		}
	}
	session := &Session{
		path:    path,
		token:   randomToken(),
		csrf:    randomToken(),
		issued:  now,
		expires: now.Add(sessionLifetime),
	}
	if err := session.write(); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Session) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

func (s *Session) CSRF() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.csrf
}

// Matches reports whether token is the current secret or the previous one
// still inside the overlap window. An expired session matches nothing.
func (s *Session) Matches(token string) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if !s.expires.IsZero() && now.After(s.expires) {
		return false
	}
	if secureCompare(token, s.token) {
		return true
	}
	if s.previous != "" && !s.previousUntil.IsZero() && !now.After(s.previousUntil) && secureCompare(token, s.previous) {
		return true
	}
	return false
}

// Maintain rotates the token after sessionRotateEvery and returns the token
// a response should set. CSRF is left alone. changed is false when the file
// could not be updated, so the previous token stays in force.
func (s *Session) Maintain(now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	if !s.expires.IsZero() && now.After(s.expires) {
		if err := s.hardRotate(now); err != nil {
			return s.token, false
		}
		return s.token, true
	}
	if s.issued.IsZero() || now.Sub(s.issued) < sessionRotateEvery {
		// Daily use inside the rotation window still slides. A fixed 14-day
		// clock signed out tabs that had been open the whole time.
		if s.expires.IsZero() || s.expires.Sub(now) >= 24*time.Hour {
			return s.token, false
		}
		oldExpires := s.expires
		s.expires = now.Add(sessionLifetime)
		if err := s.write(); err != nil {
			s.expires = oldExpires
			return s.token, false
		}
		return s.token, true
	}
	previous := s.token
	until := now.Add(sessionOverlap)
	next := randomToken()
	oldToken, oldPrev, oldUntil, oldIssued, oldExpires := s.token, s.previous, s.previousUntil, s.issued, s.expires
	s.token = next
	s.previous = previous
	s.previousUntil = until
	s.issued = now
	s.expires = now.Add(sessionLifetime)
	if err := s.write(); err != nil {
		s.token, s.previous, s.previousUntil, s.issued, s.expires = oldToken, oldPrev, oldUntil, oldIssued, oldExpires
		return s.token, false
	}
	return s.token, true
}

func (s *Session) hardRotate(now time.Time) error {
	s.token = randomToken()
	s.csrf = randomToken()
	s.previous = ""
	s.previousUntil = time.Time{}
	s.issued = now.UTC()
	s.expires = s.issued.Add(sessionLifetime)
	return s.write()
}

func (s *Session) write() error {
	doc := sessionDocument{
		Token:         s.token,
		PreviousToken: s.previous,
		PreviousUntil: s.previousUntil,
		CSRF:          s.csrf,
		IssuedAt:      s.issued,
		ExpiresAt:     s.expires,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return writeSessionFile(s.path, raw)
}

func writeSessionFile(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
