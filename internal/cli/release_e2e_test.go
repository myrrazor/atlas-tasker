package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	mdstore "github.com/myrrazor/atlas-tasker/internal/storage/markdown"
	"github.com/myrrazor/atlas-tasker/internal/testutil"
)

func TestDualGateFlowRejectsAndCompletes(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("project", "policy", "set", "APP", "--completion-mode", "dual_gate", "--lease-ttl", "45", "--required-reviewer", "agent:reviewer-1", "--allowed-workers", "agent:builder-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Flow", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "claim", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "move", "APP-1", "ready", "--actor", "agent:builder-1")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "agent:builder-1")
	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "reject", "APP-1", "--actor", "agent:reviewer-1", "--reason", "missing test coverage")

	history := must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "ticket.rejected") || !strings.Contains(history, "missing test coverage") {
		t.Fatalf("history should keep reject event and reason: %s", history)
	}

	must("ticket", "request-review", "APP-1", "--actor", "agent:builder-1")
	must("ticket", "approve", "APP-1", "--actor", "agent:reviewer-1")
	ownerQueue := must("owner-queue", "--pretty")
	if !strings.Contains(ownerQueue, "APP-1") {
		t.Fatalf("owner queue missing approved dual_gate ticket: %s", ownerQueue)
	}
	if _, err := runCLI(t, "ticket", "complete", "APP-1", "--actor", "agent:reviewer-1"); err == nil {
		t.Fatal("expected reviewer completion to fail in dual_gate")
	}
	must("ticket", "complete", "APP-1", "--actor", "human:owner")
	inspect := must("inspect", "APP-1", "--actor", "human:owner", "--json")
	if !strings.Contains(inspect, "\"status\": \"done\"") {
		t.Fatalf("inspect should show done after owner completion: %s", inspect)
	}
}

func TestExpiredClaimCanBeSweptAndReclaimed(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Lease", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "claim", "APP-1", "--actor", "agent:builder-1")

	store := mdstore.TicketStore{RootDir: mustGetwd(t)}
	ticket, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("load claimed ticket: %v", err)
	}
	ticket.Lease.ExpiresAt = time.Now().UTC().Add(-2 * time.Hour)
	ticket.Lease.LastHeartbeatAt = ticket.Lease.ExpiresAt
	ticket.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour)
	if err := store.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("persist expired lease: %v", err)
	}

	sweep := must("sweep", "--actor", "human:owner", "--reason", "cleanup")
	if !strings.Contains(sweep, "expired 1 lease(s)") {
		t.Fatalf("unexpected sweep output: %s", sweep)
	}
	must("ticket", "claim", "APP-1", "--actor", "agent:builder-2")
	who := must("who", "--json")
	if !strings.Contains(who, "agent:builder-2") || strings.Contains(who, "agent:builder-1") {
		t.Fatalf("who should show only the reclaimed lease holder: %s", who)
	}
}

func TestPolicyPrecedenceAcrossProjectEpicAndTicket(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("project", "policy", "set", "APP", "--completion-mode", "owner_gate", "--lease-ttl", "45", "--required-reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Epic", "--type", "epic", "--actor", "human:owner")
	must("ticket", "policy", "set", "APP-1", "--completion-mode", "dual_gate", "--required-reviewer", "agent:reviewer-2", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Child", "--type", "task", "--parent", "APP-1", "--actor", "human:owner")

	policyJSON := must("ticket", "policy", "get", "APP-2", "--json")
	var payload struct {
		EffectivePolicy struct {
			CompletionMode   string `json:"completion_mode"`
			RequiredReviewer string `json:"required_reviewer"`
		} `json:"effective_policy"`
	}
	if err := json.Unmarshal([]byte(policyJSON), &payload); err != nil {
		t.Fatalf("parse ticket policy output: %v\n%s", err, policyJSON)
	}
	if payload.EffectivePolicy.CompletionMode != string(contracts.CompletionModeDualGate) || payload.EffectivePolicy.RequiredReviewer != "agent:reviewer-2" {
		t.Fatalf("unexpected effective policy from project+epic inheritance: %#v", payload.EffectivePolicy)
	}

	must("ticket", "policy", "set", "APP-2", "--completion-mode", "review_gate", "--actor", "human:owner")
	policyJSON = must("ticket", "policy", "get", "APP-2", "--json")
	if err := json.Unmarshal([]byte(policyJSON), &payload); err != nil {
		t.Fatalf("parse ticket override output: %v\n%s", err, policyJSON)
	}
	if payload.EffectivePolicy.CompletionMode != string(contracts.CompletionModeReviewGate) {
		t.Fatalf("ticket override should win precedence: %#v", payload.EffectivePolicy)
	}
}

func TestFixtureUpgradeRepairReindexAndIntegrations(t *testing.T) {
	root := t.TempDir()
	if err := testutil.CopyDir(testutil.FixturePath("app_sample"), root); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir fixture: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	doctor := must("doctor", "--repair", "--pretty")
	if !strings.Contains(doctor, "doctor ok:") {
		t.Fatalf("doctor should pass on repaired fixture: %s", doctor)
	}
	history := must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "\"schema_version\": 1") {
		t.Fatalf("fixture history should keep legacy schema event: %s", history)
	}
	must("reindex")
	board := must("board", "--project", "APP", "--pretty")
	if !strings.Contains(board, "APP-1") {
		t.Fatalf("board should survive fixture reindex: %s", board)
	}
	must("integrations", "install", "codex")
	must("integrations", "install", "claude")
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md after install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md after install: %v", err)
	}
}

func TestDoctorRepairIsIdempotentAfterJournalReplayAndProjectionCorruption(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Repair me", "--type", "task", "--actor", "human:owner")

	root := mustGetwd(t)
	store := mdstore.TicketStore{RootDir: root}
	ticket, err := store.GetTicket(context.Background(), "APP-1")
	if err != nil {
		t.Fatalf("load ticket: %v", err)
	}
	now := time.Now().UTC()
	ticket.Status = contracts.StatusReady
	ticket.UpdatedAt = now
	if err := store.UpdateTicket(context.Background(), ticket); err != nil {
		t.Fatalf("update ticket: %v", err)
	}
	journal := service.MutationJournal{Root: root, Clock: func() time.Time { return now }}
	entry := service.MutationJournalEntry{
		Purpose:       "repair replay",
		CanonicalKind: "ticket_snapshot",
		Event: contracts.Event{
			EventID:       2,
			Timestamp:     now,
			Actor:         contracts.Actor("human:owner"),
			Type:          contracts.EventTicketUpdated,
			Project:       "APP",
			TicketID:      "APP-1",
			Payload:       ticket,
			SchemaVersion: contracts.CurrentSchemaVersion,
		},
		Stage: service.MutationStageCanonicalWritten,
	}
	if err := testutil.SeedPendingMutationJournal(root, journal, entry); err != nil {
		t.Fatalf("seed pending journal: %v", err)
	}
	if err := testutil.CorruptProjection(root); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}

	first := must("doctor", "--repair", "--json")
	second := must("doctor", "--repair", "--json")
	if !strings.Contains(first, "\"repair_pending\": 1") {
		t.Fatalf("expected first repair to see one pending journal entry: %s", first)
	}
	if !strings.Contains(second, "\"repair_pending\": 0") {
		t.Fatalf("expected second repair to be idempotent: %s", second)
	}

	history := must("ticket", "history", "APP-1", "--json")
	if strings.Count(history, "\"event_id\": 2") != 1 {
		t.Fatalf("expected replayed event to exist exactly once: %s", history)
	}
	board := must("board", "--pretty")
	if !strings.Contains(board, "APP-1") {
		t.Fatalf("expected board to recover after repair: %s", board)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return cwd
}
