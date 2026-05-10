package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type AgentAutoMode string

const (
	AgentAutoModeNotify  AgentAutoMode = "notify"
	AgentAutoModeCommand AgentAutoMode = "command"
)

type AgentAutoConfig struct {
	AgentID   string          `json:"agent_id"`
	Mode      AgentAutoMode   `json:"mode"`
	Argv      []string        `json:"argv,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
	UpdatedBy contracts.Actor `json:"updated_by,omitempty"`
}

type AgentWakeupState string

const (
	AgentWakeupPending  AgentWakeupState = "pending"
	AgentWakeupAcked    AgentWakeupState = "acked"
	AgentWakeupLaunched AgentWakeupState = "launched"
	AgentWakeupFailed   AgentWakeupState = "failed"
)

type AgentWakeup struct {
	WakeupID        string            `json:"wakeup_id"`
	TicketID        string            `json:"ticket_id"`
	BlockerTicketID string            `json:"blocker_ticket_id"`
	Actor           contracts.Actor   `json:"actor"`
	AgentID         string            `json:"agent_id"`
	State           AgentWakeupState  `json:"state"`
	Mode            AgentAutoMode     `json:"mode"`
	Reason          string            `json:"reason"`
	Command         []string          `json:"command,omitempty"`
	ProcessID       int               `json:"process_id,omitempty"`
	Error           string            `json:"error,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	AckedAt         time.Time         `json:"acked_at,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type AgentWakeupStore struct {
	Root string
}

func (s AgentWakeupStore) SaveWakeup(wakeup AgentWakeup) error {
	if strings.TrimSpace(wakeup.WakeupID) == "" {
		return fmt.Errorf("wakeup_id is required")
	}
	if err := os.MkdirAll(agentWakeupsDir(s.Root), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(wakeup, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.wakeupPath(wakeup.WakeupID), append(raw, '\n'), 0o644)
}

func (s AgentWakeupStore) LoadWakeup(id string) (AgentWakeup, error) {
	raw, err := os.ReadFile(s.wakeupPath(id))
	if err != nil {
		return AgentWakeup{}, err
	}
	var wakeup AgentWakeup
	if err := json.Unmarshal(raw, &wakeup); err != nil {
		return AgentWakeup{}, err
	}
	return wakeup, nil
}

func (s AgentWakeupStore) ListWakeups(agentID string) ([]AgentWakeup, error) {
	entries, err := os.ReadDir(agentWakeupsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []AgentWakeup{}, nil
		}
		return nil, err
	}
	items := make([]AgentWakeup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		item, err := s.LoadWakeup(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(agentID) != "" && item.AgentID != strings.TrimSpace(agentID) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].WakeupID < items[j].WakeupID
	})
	return items, nil
}

func (s AgentWakeupStore) Exists(id string) bool {
	_, err := os.Stat(s.wakeupPath(id))
	return err == nil
}

func (s AgentWakeupStore) wakeupPath(id string) string {
	return filepath.Join(agentWakeupsDir(s.Root), safeFileStem(id)+".json")
}

type AgentAutoStore struct {
	Root string
}

func (s AgentAutoStore) SaveConfig(config AgentAutoConfig) error {
	config.AgentID = strings.TrimSpace(config.AgentID)
	if config.AgentID == "" {
		return fmt.Errorf("agent_id is required")
	}
	if err := validateAgentAutoConfig(config); err != nil {
		return err
	}
	if err := os.MkdirAll(agentAutoDir(s.Root), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(agentAutoDir(s.Root), safeFileStem(config.AgentID)+".json"), append(raw, '\n'), 0o644)
}

func (s AgentAutoStore) LoadConfig(agentID string) (AgentAutoConfig, error) {
	raw, err := os.ReadFile(filepath.Join(agentAutoDir(s.Root), safeFileStem(agentID)+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return AgentAutoConfig{AgentID: strings.TrimSpace(agentID), Mode: AgentAutoModeNotify}, nil
		}
		return AgentAutoConfig{}, err
	}
	var config AgentAutoConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return AgentAutoConfig{}, err
	}
	if config.Mode == "" {
		config.Mode = AgentAutoModeNotify
	}
	return config, validateAgentAutoConfig(config)
}

func (s AgentAutoStore) DeleteConfig(agentID string) error {
	err := os.Remove(filepath.Join(agentAutoDir(s.Root), safeFileStem(agentID)+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func validateAgentAutoConfig(config AgentAutoConfig) error {
	switch config.Mode {
	case "", AgentAutoModeNotify:
		return nil
	case AgentAutoModeCommand:
		if len(config.Argv) == 0 || strings.TrimSpace(config.Argv[0]) == "" {
			return fmt.Errorf("command mode requires at least one argv value")
		}
		if isShellInterpreter(config.Argv[0]) {
			return fmt.Errorf("command mode refuses shell interpreters; provide the agent executable and arguments directly")
		}
		return nil
	default:
		return fmt.Errorf("invalid agent auto mode: %s", config.Mode)
	}
}

func (s *QueryService) AgentWakeups(ctx context.Context, agentID string) ([]AgentWakeup, error) {
	return AgentWakeupStore{Root: s.Root}.ListWakeups(agentID)
}

func (s *QueryService) AgentWakeup(ctx context.Context, wakeupID string) (AgentWakeup, error) {
	return AgentWakeupStore{Root: s.Root}.LoadWakeup(wakeupID)
}

func (s *QueryService) AgentAutoStatus(ctx context.Context, agentID string) (AgentAutoConfig, error) {
	if strings.TrimSpace(agentID) == "" {
		return AgentAutoConfig{}, fmt.Errorf("agent id is required")
	}
	return AgentAutoStore{Root: s.Root}.LoadConfig(agentID)
}

func (s *ActionService) SetAgentAuto(ctx context.Context, agentID string, mode AgentAutoMode, argv []string, actor contracts.Actor, reason string) (AgentAutoConfig, error) {
	return withWriteLock(ctx, s.LockManager, "set agent auto", func(ctx context.Context) (AgentAutoConfig, error) {
		if actor != contracts.Actor("human:owner") {
			return AgentAutoConfig{}, apperr.New(apperr.CodePermissionDenied, "agent auto mode can only be configured by human:owner")
		}
		if strings.TrimSpace(reason) == "" {
			return AgentAutoConfig{}, apperr.New(apperr.CodeInvalidInput, "agent auto mode requires a reason")
		}
		config := AgentAutoConfig{AgentID: strings.TrimSpace(agentID), Mode: mode, Argv: cleanArgv(argv), UpdatedAt: s.now(), UpdatedBy: actor}
		if config.Mode == "" {
			config.Mode = AgentAutoModeNotify
		}
		if err := validateAgentAutoConfig(config); err != nil {
			return AgentAutoConfig{}, apperr.New(apperr.CodeInvalidInput, err.Error())
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, config.UpdatedAt, actor, reason, contracts.EventAgentUpdated, "", map[string]any{"agent_id": config.AgentID, "auto": config})
		if err != nil {
			return AgentAutoConfig{}, err
		}
		if err := s.commitMutation(ctx, "set agent auto", "agent_auto", event, func(context.Context) error {
			return AgentAutoStore{Root: s.Root}.SaveConfig(config)
		}); err != nil {
			return AgentAutoConfig{}, err
		}
		return config, nil
	})
}

func (s *ActionService) DisableAgentAuto(ctx context.Context, agentID string, actor contracts.Actor, reason string) (AgentAutoConfig, error) {
	return withWriteLock(ctx, s.LockManager, "disable agent auto", func(ctx context.Context) (AgentAutoConfig, error) {
		if actor != contracts.Actor("human:owner") {
			return AgentAutoConfig{}, apperr.New(apperr.CodePermissionDenied, "agent auto mode can only be configured by human:owner")
		}
		if strings.TrimSpace(reason) == "" {
			return AgentAutoConfig{}, apperr.New(apperr.CodeInvalidInput, "agent auto mode requires a reason")
		}
		config := AgentAutoConfig{AgentID: strings.TrimSpace(agentID), Mode: AgentAutoModeNotify, UpdatedAt: s.now(), UpdatedBy: actor}
		event, err := s.newEvent(ctx, workspaceProjectKey, config.UpdatedAt, actor, reason, contracts.EventAgentUpdated, "", map[string]any{"agent_id": config.AgentID, "auto": config})
		if err != nil {
			return AgentAutoConfig{}, err
		}
		store := AgentAutoStore{Root: s.Root}
		if err := s.commitMutation(ctx, "disable agent auto", "agent_auto", event, func(context.Context) error {
			if err := store.DeleteConfig(config.AgentID); err != nil {
				return err
			}
			return store.SaveConfig(config)
		}); err != nil {
			return AgentAutoConfig{}, err
		}
		return config, nil
	})
}

func (s *ActionService) AckAgentWakeup(ctx context.Context, wakeupID string, actor contracts.Actor, reason string) (AgentWakeup, error) {
	return withWriteLock(ctx, s.LockManager, "ack agent wakeup", func(ctx context.Context) (AgentWakeup, error) {
		if !actor.IsValid() {
			return AgentWakeup{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return AgentWakeup{}, apperr.New(apperr.CodeInvalidInput, "ack requires a reason")
		}
		store := AgentWakeupStore{Root: s.Root}
		wakeup, err := store.LoadWakeup(wakeupID)
		if err != nil {
			return AgentWakeup{}, err
		}
		wakeup.State = AgentWakeupAcked
		wakeup.AckedAt = s.now()
		event, err := s.newEvent(ctx, workspaceProjectKey, wakeup.AckedAt, actor, reason, contracts.EventAgentUpdated, wakeup.TicketID, map[string]any{"wakeup": wakeup, "acked": true})
		if err != nil {
			return AgentWakeup{}, err
		}
		if err := s.commitMutation(ctx, "ack agent wakeup", "agent_wakeup", event, func(context.Context) error {
			return store.SaveWakeup(wakeup)
		}); err != nil {
			return AgentWakeup{}, err
		}
		return wakeup, nil
	})
}

func (s *ActionService) emitAgentWakeups(ctx context.Context, event contracts.Event) {
	if !ticketCompletionEvent(event) || strings.TrimSpace(event.TicketID) == "" {
		return
	}
	completed, err := s.Tickets.GetTicket(ctx, event.TicketID)
	if err != nil || completed.Status != contracts.StatusDone {
		return
	}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{IncludeArchived: false})
	if err != nil {
		return
	}
	queries := NewQueryService(s.Root, s.Projects, s.Tickets, s.Events, s.Projection, s.Clock)
	for _, candidate := range tickets {
		if candidate.ID == completed.ID || !containsString(candidate.BlockedBy, completed.ID) || contracts.IsTerminalStatus(candidate.Status) {
			continue
		}
		if len(candidate.BlockedBy) == 0 || len(candidate.Assignee) == 0 || !strings.HasPrefix(string(candidate.Assignee), "agent:") {
			continue
		}
		blockers, err := unresolvedBlockersFromStore(ctx, s.Tickets, candidate)
		if err != nil || len(blockers) > 0 {
			continue
		}
		view, err := queries.AgentAvailable(ctx, candidate.Assignee)
		if err != nil || !agentWorkViewContains(view, candidate.ID) {
			continue
		}
		_ = s.createAgentWakeup(ctx, candidate, completed.ID, event)
	}
}

func (s *ActionService) createAgentWakeup(ctx context.Context, ticket contracts.TicketSnapshot, blockerID string, cause contracts.Event) error {
	store := AgentWakeupStore{Root: s.Root}
	wakeupID := fmt.Sprintf("wakeup_%s_after_%s", safeFileStem(ticket.ID), safeFileStem(blockerID))
	if store.Exists(wakeupID) {
		return nil
	}
	agentID := agentIDFromActor(ticket.Assignee)
	auto, err := AgentAutoStore{Root: s.Root}.LoadConfig(agentID)
	if err != nil {
		auto = AgentAutoConfig{AgentID: agentID, Mode: AgentAutoModeNotify}
	}
	wakeup := AgentWakeup{
		WakeupID:        wakeupID,
		TicketID:        ticket.ID,
		BlockerTicketID: blockerID,
		Actor:           ticket.Assignee,
		AgentID:         agentID,
		State:           AgentWakeupPending,
		Mode:            auto.Mode,
		Reason:          "dependency completed; assigned work is available",
		CreatedAt:       s.now(),
		Metadata: map[string]string{
			"causation_event_id": fmt.Sprintf("%d", cause.EventID),
		},
	}
	event, err := s.newEvent(ctx, ticket.Project, wakeup.CreatedAt, contracts.Actor("agent:automation"), wakeup.Reason, contracts.EventAgentWorkAvailable, ticket.ID, map[string]any{"wakeup": wakeup})
	if err != nil {
		return err
	}
	if err := s.commitMutation(ctx, "agent work wakeup", "agent_wakeup", event, func(context.Context) error {
		return store.SaveWakeup(wakeup)
	}); err != nil {
		return err
	}
	if auto.Mode == AgentAutoModeCommand {
		updated := launchAgentWakeupCommand(ctx, wakeup, auto)
		_ = store.SaveWakeup(updated)
	}
	return nil
}

func ticketCompletionEvent(event contracts.Event) bool {
	switch event.Type {
	case contracts.EventTicketMoved, contracts.EventTicketApproved, contracts.EventTicketClosed:
		return true
	default:
		return false
	}
}

func agentWorkViewContains(view AgentWorkView, ticketID string) bool {
	for _, entry := range view.Available {
		if entry.Ticket.ID == ticketID {
			return true
		}
	}
	return false
}

func launchAgentWakeupCommand(ctx context.Context, wakeup AgentWakeup, config AgentAutoConfig) AgentWakeup {
	wakeup.Command = substituteWakeupArgv(config.Argv, wakeup)
	if err := validateAgentAutoConfig(AgentAutoConfig{AgentID: config.AgentID, Mode: AgentAutoModeCommand, Argv: wakeup.Command}); err != nil {
		wakeup.State = AgentWakeupFailed
		wakeup.Error = err.Error()
		return wakeup
	}
	cmd := exec.CommandContext(ctx, wakeup.Command[0], wakeup.Command[1:]...)
	if err := cmd.Start(); err != nil {
		wakeup.State = AgentWakeupFailed
		wakeup.Error = err.Error()
		return wakeup
	}
	wakeup.State = AgentWakeupLaunched
	wakeup.ProcessID = cmd.Process.Pid
	_ = cmd.Process.Release()
	return wakeup
}

func substituteWakeupArgv(argv []string, wakeup AgentWakeup) []string {
	out := make([]string, 0, len(argv))
	replacements := map[string]string{
		"{ticket_id}":  wakeup.TicketID,
		"{blocker_id}": wakeup.BlockerTicketID,
		"{agent_id}":   wakeup.AgentID,
		"{actor}":      string(wakeup.Actor),
		"{wakeup_id}":  wakeup.WakeupID,
	}
	for _, arg := range argv {
		item := arg
		for from, to := range replacements {
			item = strings.ReplaceAll(item, from, to)
		}
		out = append(out, item)
	}
	return out
}

func cleanArgv(argv []string) []string {
	out := make([]string, 0, len(argv))
	for _, arg := range argv {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}

func isShellInterpreter(path string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	switch base {
	case "sh", "bash", "zsh", "fish", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return true
	default:
		return false
	}
}

func safeFileStem(raw string) string {
	raw = strings.TrimSpace(raw)
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "item"
	}
	return b.String()
}

func agentWakeupsDir(root string) string {
	return filepath.Join(storage.TrackerDir(root), "runtime", "agent-wakeups")
}

func agentAutoDir(root string) string {
	return filepath.Join(storage.TrackerDir(root), "agent-auto")
}
