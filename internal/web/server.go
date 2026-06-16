package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

const (
	sessionCookie = "atlas_web_session"
	csrfHeader    = "X-Atlas-CSRF"
)

type Services struct {
	Actions *service.ActionService
	Queries *service.QueryService
}

type Config struct {
	Root       string
	Workspace  string
	Host       string
	Port       int
	Project    string
	Actor      contracts.Actor
	ReadOnly   bool
	UnsafeHost bool
	TokenMode  string
	Token      string
	CSRFToken  string
	Clock      func() time.Time
}

type Server struct {
	cfg       Config
	actions   *service.ActionService
	queries   *service.QueryService
	templates *template.Template
	static    fs.FS
	startedAt time.Time
}

type contextKey string

const requestIDKey contextKey = "request_id"

func NewServer(services Services, cfg Config) (*Server, error) {
	if services.Actions == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "web actions service is required")
	}
	if services.Queries == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "web query service is required")
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.TokenMode == "" {
		cfg.TokenMode = "random"
	}
	if cfg.Actor == "" {
		cfg.Actor = contracts.Actor("human:owner")
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "workspace"
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.TokenMode != "random" {
		return nil, apperr.New(apperr.CodeInvalidInput, "only token-mode=random is supported")
	}
	if !isLoopbackHost(cfg.Host) && !cfg.UnsafeHost {
		return nil, apperr.New(apperr.CodePermissionDenied, "non-loopback web host requires --unsafe-host")
	}
	if cfg.Token == "" {
		cfg.Token = randomToken()
	}
	if cfg.CSRFToken == "" {
		cfg.CSRFToken = randomToken()
	}
	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	static, err := staticFS()
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:       cfg,
		actions:   services.Actions,
		queries:   services.Queries,
		templates: templates,
		static:    static,
		startedAt: cfg.Clock(),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.static))))
	mux.HandleFunc("/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/board", s.handleBoardAPI)
	mux.HandleFunc("/api/tickets/", s.handleTicketAPI)
	mux.HandleFunc("/actions/tickets/create", s.handleCreateTicket)
	mux.HandleFunc("/actions/tickets/", s.handleTicketAction)
	mux.HandleFunc("/new-ticket", s.handleNewTicket)
	mux.HandleFunc("/tickets/", s.handleTicketPage)
	mux.HandleFunc("/board", s.handleBoard)
	mux.HandleFunc("/", s.handleRoot)
	return s.security(mux)
}

func (s *Server) RuntimeState(port int) RuntimeState {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(s.cfg.Host, strconv.Itoa(port)), Path: "/board"}
	return RuntimeState{
		Host:      s.cfg.Host,
		Port:      port,
		URL:       u.String(),
		PID:       os.Getpid(),
		Project:   strings.TrimSpace(s.cfg.Project),
		Actor:     string(s.cfg.Actor),
		ReadOnly:  s.cfg.ReadOnly,
		StartedAt: s.startedAt.UTC(),
	}
}

func (s *Server) SessionURL(port int) string {
	state := s.RuntimeState(port)
	u, _ := url.Parse(state.URL)
	q := u.Query()
	q.Set("token", s.cfg.Token)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	server := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() {
		err := server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return <-done
	case err := <-done:
		return err
	}
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := randomToken()[:16]
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)
		s.writeSecurityHeaders(w, r)
		if r.Method == http.MethodOptions {
			http.Error(w, "CORS is not enabled", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/favicon.ico" || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.validSession(w, r) {
			return
		}
		if isMutation(r.Method) {
			if err := s.validateMutation(r); err != nil {
				s.writeError(w, r, err, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeSecurityHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !strings.HasPrefix(r.URL.Path, "/static/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.Header().Set("X-Atlas-Request-ID", requestIDFromContext(r.Context()))
}

func (s *Server) validSession(w http.ResponseWriter, r *http.Request) bool {
	if token := r.URL.Query().Get("token"); secureCompare(token, s.cfg.Token) {
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    s.cfg.Token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		clean := *r.URL
		q := clean.Query()
		q.Del("token")
		clean.RawQuery = q.Encode()
		http.Redirect(w, r, clean.String(), http.StatusSeeOther)
		return false
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || !secureCompare(cookie.Value, s.cfg.Token) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("Atlas web session required. Start with `tracker web serve --open` to open a session URL.\n"))
		return false
	}
	return true
}

func (s *Server) validateMutation(r *http.Request) error {
	if !s.sameOrigin(r.Header.Get("Origin"), r.Host) {
		return apperr.New(apperr.CodePermissionDenied, "cross-origin mutation rejected")
	}
	if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" && !s.sameOrigin(ref, r.Host) {
		return apperr.New(apperr.CodePermissionDenied, "cross-origin referer rejected")
	}
	if err := r.ParseForm(); err != nil {
		return apperr.Wrap(apperr.CodeInvalidInput, err, "parse form")
	}
	token := r.Header.Get(csrfHeader)
	if token == "" {
		token = r.Form.Get("csrf_token")
	}
	if !secureCompare(token, s.cfg.CSRFToken) {
		return apperr.New(apperr.CodePermissionDenied, "invalid csrf token")
	}
	return nil
}

func (s *Server) sameOrigin(raw string, host string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, host) && (u.Scheme == "http" || u.Scheme == "https")
}

func (s *Server) mutationContext(r *http.Request, actor contracts.Actor) context.Context {
	return service.WithEventMetadata(r.Context(), service.EventMetaContext{
		Surface:       contracts.EventSurfaceWeb,
		CorrelationID: requestIDFromContext(r.Context()),
		RootActor:     actor,
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, kind string, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"format_version": "v1",
		"kind":           kind,
		"generated_at":   s.cfg.Clock().UTC(),
		"payload":        payload,
	})
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(apperr.Envelope(err))
		return
	}
	http.Error(w, err.Error(), status)
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	if value == "" {
		return "request"
	}
	return value
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") || strings.Contains(r.Header.Get("X-Atlas-Request"), "fetch")
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func secureCompare(left string, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func randomToken() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
