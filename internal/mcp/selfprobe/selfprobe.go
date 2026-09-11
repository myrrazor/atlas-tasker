package selfprobe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

const ProtocolVersion = "2025-11-25"

// Options controls a canonical Atlas MCP self-probe (AT114-201).
type Options struct {
	// Command is the exact registered server process.
	Command adapter.Command
	// Mutate runs one workflow write in the probed workspace. Use only against
	// a disposable workspace; adapter Verify never sets this.
	Mutate bool
	// Actor and Reason are used only when Mutate is true.
	Actor  string
	Reason string
}

// Report is the evidence produced by a self-probe.
type Report struct {
	Initialized     bool
	ServerName      string
	Tools           []string
	DashboardOK     bool
	BoardOK         bool
	MutationOK      bool
	HighImpactFound []string
	ShutdownOK      bool
	Detail          string
}

func (r Report) Passed() bool {
	return r.Initialized && r.DashboardOK && r.BoardOK && len(r.HighImpactFound) == 0
}

var highImpactDenied = []string{
	"atlas.change.merge",
	"atlas.change.review_request",
	"atlas.gate.waive",
	"atlas.sync.pull",
	"atlas.sync.push",
	"atlas.bundle.import",
	"atlas.import.apply",
	"atlas.archive.apply",
	"atlas.archive.restore",
	"atlas.compact",
	"atlas.worktree.cleanup",
	"atlas.dispatch.run",
}

// Run starts the exact registered command and performs the AT114-201 probe.
func Run(ctx context.Context, opts Options) (report Report, err error) {
	if err := opts.Command.Validate(); err != nil {
		return Report{}, err
	}
	if opts.Command.Purpose != adapter.CommandPurposeProbe {
		return Report{}, fmt.Errorf("self-probe command purpose must be probe, got %s", opts.Command.Purpose)
	}
	timeout := opts.Command.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, opts.Command.Executable, opts.Command.Args...)
	if opts.Command.Dir != "" {
		cmd.Dir = opts.Command.Dir
	}
	cmd.Env = []string{"NO_COLOR=1", "TERM=dumb", "LC_ALL=C", "HOME=" + os.Getenv("HOME"), "PATH=" + os.Getenv("PATH")}
	for _, entry := range opts.Command.Env {
		cmd.Env = append(cmd.Env, entry.Name+"="+entry.Value)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Report{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Report{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Report{}, fmt.Errorf("start registered server: %w", err)
	}

	sess := &session{stdin: stdin, reader: bufio.NewReader(stdout), timeout: timeout}
	defer func() {
		_ = stdin.Close()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
			report.ShutdownOK = true
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		if report.Detail == "" && stderr.Len() > 0 {
			report.Detail = truncate(stderr.String(), 800)
		}
	}()

	init, err := sess.request(runCtx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "atlas-selfprobe", "version": "v1"},
	})
	if err != nil {
		report.Detail = err.Error()
		return report, nil
	}
	result, _ := init["result"].(map[string]any)
	if result == nil {
		report.Detail = "initialize returned no result"
		return report, nil
	}
	if result["protocolVersion"] != ProtocolVersion {
		report.Detail = fmt.Sprintf("unexpected protocol %v", result["protocolVersion"])
		return report, nil
	}
	if info, _ := result["serverInfo"].(map[string]any); info != nil {
		if name, _ := info["name"].(string); name != "" {
			report.ServerName = name
		}
	}
	if report.ServerName != "atlas-tasker" {
		report.Detail = fmt.Sprintf("unexpected server %q", report.ServerName)
		return report, nil
	}
	if err := sess.notify(runCtx, "notifications/initialized", nil); err != nil {
		report.Detail = err.Error()
		return report, nil
	}
	report.Initialized = true

	listed, err := sess.request(runCtx, "tools/list", map[string]any{})
	if err != nil {
		report.Detail = err.Error()
		return report, nil
	}
	tools := toolNames(listed)
	report.Tools = tools
	for _, name := range highImpactDenied {
		for _, have := range tools {
			if have == name {
				report.HighImpactFound = append(report.HighImpactFound, name)
			}
		}
	}
	if !contains(tools, "atlas.dashboard") || !contains(tools, "atlas.board") {
		report.Detail = "workflow read tools missing"
		return report, nil
	}
	if contains(tools, "atlas.ticket.create") == opts.Mutate && opts.Mutate {
		// mutation path checked below
	}

	dash, err := sess.call(runCtx, "atlas.dashboard", map[string]any{})
	if err == nil && toolOK(dash) {
		report.DashboardOK = true
	} else if err != nil {
		report.Detail = "atlas.dashboard: " + err.Error()
	} else {
		report.Detail = "atlas.dashboard failed"
	}
	board, err := sess.call(runCtx, "atlas.board", map[string]any{})
	if err == nil && toolOK(board) {
		report.BoardOK = true
	} else if err != nil {
		report.Detail = "atlas.board: " + err.Error()
	} else {
		report.Detail = "atlas.board failed"
	}

	if opts.Mutate && contains(tools, "atlas.project.create") {
		actor := strings.TrimSpace(opts.Actor)
		if actor == "" {
			actor = "human:owner"
		}
		reason := strings.TrimSpace(opts.Reason)
		if reason == "" {
			reason = "AT114-201 self-probe mutation"
		}
		created, err := sess.call(runCtx, "atlas.project.create", map[string]any{
			"key":  "PRB",
			"name": "Selfprobe",
		})
		if err == nil && toolOK(created) {
			report.MutationOK = true
		} else if contains(tools, "atlas.ticket.create") {
			ticket, terr := sess.call(runCtx, "atlas.ticket.create", map[string]any{
				"project": "PRB",
				"title":   "selfprobe",
				"type":    "task",
				"actor":   actor,
				"reason":  reason,
			})
			report.MutationOK = terr == nil && toolOK(ticket)
		}
	}

	return report, nil
}

type session struct {
	stdin   io.WriteCloser
	reader  *bufio.Reader
	timeout time.Duration
	nextID  int
	mu      sync.Mutex
}

func (s *session) request(ctx context.Context, method string, params any) (map[string]any, error) {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := s.write(msg); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(s.timeout)
	if t, ok := ctx.Deadline(); ok && t.Before(deadline) {
		deadline = t
	}
	for time.Now().Before(deadline) {
		raw, err := s.readLine(deadline)
		if err != nil {
			return nil, err
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			continue
		}
		if decoded["id"] == nil {
			continue
		}
		switch v := decoded["id"].(type) {
		case float64:
			if int(v) != id {
				continue
			}
		case json.Number:
			if n, _ := v.Int64(); int(n) != id {
				continue
			}
		}
		return decoded, nil
	}
	return nil, fmt.Errorf("timed out waiting for %s", method)
}

func (s *session) notify(_ context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return s.write(msg)
}

func (s *session) call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	return s.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
}

func (s *session) write(msg map[string]any) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = s.stdin.Write(append(raw, '\n'))
	return err
}

func (s *session) readLine(deadline time.Time) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := s.reader.ReadBytes('\n')
		ch <- result{line, err}
	}()
	remain := time.Until(deadline)
	if remain <= 0 {
		return nil, fmt.Errorf("read deadline")
	}
	select {
	case out := <-ch:
		return bytes.TrimSpace(out.line), out.err
	case <-time.After(remain):
		return nil, fmt.Errorf("read deadline")
	}
}

func toolNames(resp map[string]any) []string {
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		return nil
	}
	raw, _ := result["tools"].([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		obj, _ := item.(map[string]any)
		if obj == nil {
			continue
		}
		if name, _ := obj["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func toolOK(resp map[string]any) bool {
	if resp["error"] != nil {
		return false
	}
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		return false
	}
	if errFlag, _ := result["isError"].(bool); errFlag {
		return false
	}
	if structured, _ := result["structuredContent"].(map[string]any); structured != nil {
		if ok, _ := structured["ok"].(bool); ok {
			return true
		}
		if structured["format_version"] == "v1" && structured["error"] == nil {
			return true
		}
	}
	return result["content"] != nil
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
