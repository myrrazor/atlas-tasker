package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	webui "github.com/myrrazor/atlas-tasker/internal/web"
	"github.com/spf13/cobra"
)

func newWebCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "web", Short: "Run the local browser Kanban board"}
	serve := &cobra.Command{Use: "serve", Short: "Serve the local browser Kanban board", RunE: runWebServe}
	serve.Flags().String("host", "127.0.0.1", "Host to bind; non-loopback requires --unsafe-host")
	serve.Flags().Int("port", 0, "Port to bind; 0 chooses a random free port")
	serve.Flags().String("project", "", "Default project filter")
	serve.Flags().String("actor", "human:owner", "Default mutation actor")
	serve.Flags().Bool("open", false, "Open the board in the default browser")
	serve.Flags().Bool("no-browser", false, "Do not open a browser")
	serve.Flags().Bool("read-only", false, "Disable all web mutations")
	serve.Flags().String("token-mode", "random", "Session token mode; random is the only supported mode")
	serve.Flags().Bool("unsafe-host", false, "Allow binding to a non-loopback host")

	openCmd := &cobra.Command{Use: "open", Short: "Open the last recorded local web board URL", RunE: runWebOpen}
	status := &cobra.Command{Use: "status", Short: "Show the last recorded local web board server", RunE: runWebStatus}
	addReadOutputFlags(status, &outputFlags{})
	cmd.AddCommand(serve, openCmd, status)
	return cmd
}

func runWebServe(cmd *cobra.Command, _ []string) error {
	ctx := commandContext(cmd)
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	project, _ := cmd.Flags().GetString("project")
	actorRaw, _ := cmd.Flags().GetString("actor")
	openBrowser, _ := cmd.Flags().GetBool("open")
	noBrowser, _ := cmd.Flags().GetBool("no-browser")
	readOnly, _ := cmd.Flags().GetBool("read-only")
	tokenMode, _ := cmd.Flags().GetString("token-mode")
	unsafeHost, _ := cmd.Flags().GetBool("unsafe-host")
	actor, err := workspace.queries.ResolveActor(ctx, contracts.Actor(strings.TrimSpace(actorRaw)))
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	actualPort := listener.Addr().(*net.TCPAddr).Port
	server, err := webui.NewServer(webui.Services{Actions: workspace.actions, Queries: workspace.queries}, webui.Config{
		Root:       workspace.root,
		Workspace:  filepath.Base(workspace.root),
		Host:       host,
		Port:       actualPort,
		Project:    project,
		Actor:      actor,
		ReadOnly:   readOnly,
		UnsafeHost: unsafeHost,
		TokenMode:  tokenMode,
		Clock:      defaultNow,
	})
	if err != nil {
		return err
	}
	state := server.RuntimeState(actualPort)
	if err := webui.WriteRuntimeState(workspace.root, state); err != nil {
		return err
	}
	defer func() { _ = webui.ClearRuntimeStateOwnedBy(workspace.root, os.Getpid()) }()
	sessionURL := server.SessionURL(actualPort)
	if unsafeHost {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: web board is bound to a non-loopback host; use only on trusted networks")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "serving Atlas web board at %s\n", state.URL)
	fmt.Fprintf(cmd.OutOrStdout(), "session URL: %s\n", sessionURL)
	if openBrowser && !noBrowser {
		if err := openURLFunc(sessionURL); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "open browser failed: %v\n", err)
		}
	}
	serveCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Serve(serveCtx, listener)
}

func runWebStatus(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	state, err := webui.ReadRuntimeState(workspace.root)
	if err != nil {
		return err
	}
	stateMap := map[string]any{
		"host":       state.Host,
		"port":       state.Port,
		"url":        state.URL,
		"pid":        state.PID,
		"project":    state.Project,
		"actor":      state.Actor,
		"read_only":  state.ReadOnly,
		"started_at": state.StartedAt,
		"health":     webHealth(state.URL),
	}
	md := fmt.Sprintf("## Web Board\n\n- URL: %s\n- PID: %d\n- Health: %s\n", state.URL, state.PID, stateMap["health"])
	pretty := fmt.Sprintf("web board %s pid=%d health=%s", state.URL, state.PID, stateMap["health"])
	return writeCommandOutput(cmd, stateMap, md, pretty)
}

func runWebOpen(cmd *cobra.Command, _ []string) error {
	workspace, err := openWorkspace()
	if err != nil {
		return err
	}
	defer workspace.close()
	state, err := webui.ReadRuntimeState(workspace.root)
	if err != nil {
		return err
	}
	// stale state outlives crashed/stopped servers; don't open a dead URL
	if health := webHealth(state.URL); health != "ok" {
		return fmt.Errorf("web board at %s is not running (health: %s); start it with `tracker web serve --open`", state.URL, health)
	}
	if err := openURLFunc(state.URL); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "opened %s\n", state.URL)
	return nil
}

// swappable for tests so they don't launch a real browser
var openURLFunc = openURL

func openURL(raw string) error {
	var browser *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		browser = exec.Command("open", raw)
	case "linux":
		browser = exec.Command("xdg-open", raw)
	case "windows":
		browser = exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		return fmt.Errorf("opening browsers is unsupported on %s", runtime.GOOS)
	}
	if err := browser.Start(); err != nil {
		return err
	}
	// reap the child so it doesn't linger as a zombie for the server's lifetime
	go func() { _ = browser.Wait() }()
	return nil
}

func webHealth(rawURL string) string {
	healthURL := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(healthURL, "/board") {
		healthURL = strings.TrimSuffix(healthURL, "/board")
	}
	healthURL += "/healthz"
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(healthURL)
	if err != nil {
		return "down"
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return "ok"
	}
	return resp.Status
}
