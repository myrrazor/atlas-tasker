package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/app"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type HomeConfig struct {
	Host     string
	Port     int
	Actor    contracts.Actor
	ReadOnly bool
	Token    string
	CSRF     string
	Clock    func() time.Time
}

type HomeServer struct {
	application *app.App
	cfg         HomeConfig
	token       string
	csrf        string
	templates   *template.Template
	static      fs.FS
	staticETags map[string]string
	startedAt   time.Time
}

func NewHomeServer(application *app.App, cfg HomeConfig) (*HomeServer, error) {
	if application == nil {
		return nil, apperr.New(apperr.CodeInvalidInput, "home server requires app")
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if !isLoopbackHost(cfg.Host) {
		return nil, apperr.New(apperr.CodePermissionDenied, "web host must be loopback")
	}
	if cfg.Actor == "" {
		cfg.Actor = "human:owner"
	}
	if cfg.Clock == nil {
		cfg.Clock = func() time.Time { return time.Now().UTC() }
	}
	if cfg.Token == "" {
		cfg.Token = randomToken()
	}
	if cfg.CSRF == "" {
		cfg.CSRF = randomToken()
	}
	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	static, err := staticFS()
	if err != nil {
		return nil, err
	}
	etags, err := computeStaticETags(static)
	if err != nil {
		return nil, err
	}
	return &HomeServer{
		application: application,
		cfg:         cfg,
		token:       cfg.Token,
		csrf:        cfg.CSRF,
		templates:   templates,
		static:      static,
		staticETags: etags,
		startedAt:   cfg.Clock(),
	}, nil
}

func (s *HomeServer) Handler() http.Handler {
	mux := http.NewServeMux()
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(s.static)))
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if etag, ok := s.staticETags[strings.TrimPrefix(r.URL.Path, "/static/")]; ok {
			w.Header().Set("ETag", etag)
		}
		fileServer.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/favicon.svg", http.StatusSeeOther)
	})
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/session/claim", s.handleClaim)
	mux.HandleFunc("/session/claim/", s.handleClaim)
	mux.HandleFunc("/attention", s.handleAttention)
	mux.HandleFunc("/search", s.handleSearch)
	mux.HandleFunc("/settings/agents", s.handleSettingsAgents)
	mux.HandleFunc("/settings/workspaces", s.handleSettingsWorkspaces)
	mux.HandleFunc("/settings", s.handleHomeSettings)
	mux.HandleFunc("/actions/workspaces/init", s.handleInitWorkspace)
	mux.HandleFunc("/actions/workspaces/register", s.handleRegisterWorkspace)
	mux.HandleFunc("/w/", s.handleWorkspace)
	mux.HandleFunc("/api/v1/agents", s.handleAgentsAPI)
	mux.HandleFunc("/api/v1/grants", s.handleGrantsAPI)
	mux.HandleFunc("/api/v1/workspaces/", s.handleWorkspaceAPI)
	mux.HandleFunc("/", s.handleHome)
	return s.security(mux)
}

func (s *HomeServer) Serve(ctx context.Context, ln net.Listener) error {
	if err := validateLoopbackListener(ln); err != nil {
		return err
	}
	if addr, ok := ln.Addr().(*net.TCPAddr); ok {
		s.cfg.Port = addr.Port
	}
	jobsCtx, stopJobs := context.WithCancel(ctx)
	go func() { _ = s.application.RunWorkspaceJobs(jobsCtx) }()
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
		stopJobs()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return <-done
	case err := <-done:
		stopJobs()
		return err
	}
}

func (s *HomeServer) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID))
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/session/claim") {
			w.Header().Set("Referrer-Policy", "no-referrer")
		} else {
			w.Header().Set("Referrer-Policy", "same-origin")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Atlas-Request-ID", requestID)
		if !loopbackHostAllowed(r.Host, s.cfg.Host, s.cfg.Port) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodOptions {
			http.Error(w, "CORS is not enabled", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/session/claim") {
			if isMutation(r.Method) {
				r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
				if !exactOriginAllowed(r.Header.Get("Origin"), s.cfg.Host, s.cfg.Port) {
					http.Error(w, "cross-origin claim rejected", http.StatusForbidden)
					return
				}
			} else if origin := r.Header.Get("Origin"); origin != "" && !exactOriginAllowed(origin, s.cfg.Host, s.cfg.Port) {
				http.Error(w, "cross-origin claim rejected", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if isMutation(r.Method) {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			origin := r.Header.Get("Origin")
			if !exactOriginAllowed(origin, s.cfg.Host, s.cfg.Port) {
				http.Error(w, "cross-origin mutation rejected", http.StatusForbidden)
				return
			}
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
				http.Error(w, err.Error(), http.StatusForbidden)
				return
			}
			if s.cfg.ReadOnly {
				http.Error(w, "read-only", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *HomeServer) validSession(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie(s.sessionCookieName())
	if err != nil || !secureCompare(cookie.Value, s.token) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Atlas Home session required. Open Atlas from `tracker` or `tracker serve`.\n"))
			return false
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || wantsJSON(r) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Atlas Home session required. Open Atlas from `tracker` or `tracker serve`.\n"))
			return false
		}
		s.writeClaimPage(w)
		return false
	}
	return true
}

func (s *HomeServer) writeClaimPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Atlas Home</title>
  <link rel="icon" href="/static/favicon.svg" type="image/svg+xml">
  <link rel="stylesheet" href="/static/app.css">
  <link rel="stylesheet" href="/static/home.css">
</head>
<body class="home-body">
  <main class="home-main home-sign-in">
    <p class="rail-brand"><img src="/static/brand/atlas-tasker-ascii.svg" alt="Atlas Tasker"></p>
    <h1>Sign in on this computer</h1>
    <p class="lede">Atlas Home is a local loopback app. There is no account and no password.</p>
    <p>Run <code>tracker</code> in a terminal on this machine. That command opens a one-time local sign-in. This address stays on loopback and never carries a session secret.</p>
    <p>If you already did that, this page finishes signing in automatically.</p>
  </main>
  <script src="/static/claim.js"></script>
</body>
</html>
`))
}

func (s *HomeServer) sessionCookieName() string {
	if s.cfg.Port > 0 {
		return fmt.Sprintf("%s_%d", sessionCookie, s.cfg.Port)
	}
	return sessionCookie
}

func (s *HomeServer) validateMutation(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return apperr.Wrap(apperr.CodeInvalidInput, err, "parse form")
	}
	token := r.Header.Get(csrfHeader)
	if token == "" {
		token = r.Form.Get("csrf_token")
	}
	if !secureCompare(token, s.csrf) {
		return apperr.New(apperr.CodePermissionDenied, "invalid csrf token")
	}
	return nil
}

func (s *HomeServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	for k, v := range app.HealthIdentityHeaders(s.application.Settings().InstanceID) {
		w.Header().Set(k, v)
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(app.HealthJSON(s.application.Settings().InstanceID, s.cfg.Port))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (s *HomeServer) handleClaim(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	token := ""
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		token = strings.TrimSpace(r.Form.Get("claim"))
	} else if r.Method == http.MethodGet || r.Method == http.MethodHead {
		token = strings.TrimPrefix(r.URL.Path, "/session/claim/")
		token = strings.Trim(token, "/")
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.application.ConsumeClaim(token); err != nil {
		http.Error(w, "invalid session claim", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    s.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *HomeServer) innerServer(ws *app.Workspace, project string) *Server {
	display := ws.ID
	if rec, err := s.application.WorkspaceByID(context.Background(), ws.ID); err == nil && strings.TrimSpace(rec.DisplayName) != "" {
		display = rec.DisplayName
	}
	return &Server{
		cfg: Config{
			Root:        ws.Root,
			Workspace:   firstNonEmpty(ws.ID, "workspace"),
			DisplayName: display,
			Host:        s.cfg.Host,
			Port:        s.cfg.Port,
			Project:     project,
			Actor:       s.cfg.Actor,
			ReadOnly:    s.cfg.ReadOnly,
			TokenMode:   "random",
			Token:       s.token,
			CSRFToken:   s.csrf,
			Clock:       s.cfg.Clock,
			RoutePrefix: "/w/" + ws.ID,
			BoardPath:   homeBoardPath(ws.ID, project),
			HomePath:    "/w/" + ws.ID,
		},
		actions:     ws.Actions,
		queries:     ws.Queries,
		templates:   s.templates,
		static:      s.static,
		staticETags: s.staticETags,
		startedAt:   s.startedAt,
	}
}

func (s *HomeServer) bind(r *http.Request, id string) (*app.Workspace, error) {
	return s.application.Hub().Bind(r.Context(), id)
}

func (s *HomeServer) handleAgentsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	report := s.application.ListAgentClients(r.Context())
	writeJSONHome(w, "atlas_home_agents", report)
}

func (s *HomeServer) handleGrantsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSONHome(w, "atlas_home_grants", s.application.ListPendingGrants())
}
