package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestPermissionProfileCommandsAndDispatchEnforcement(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Protected dispatch", "--type", "task", "--protected", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")

	createOut := must(
		"permission-profile", "create", "owner-ops",
		"--name", "Owner Ops",
		"--workspace-default",
		"--require-owner-for-sensitive-ops",
		"--actor", "human:owner",
		"--json",
	)
	var createView struct {
		Kind    string `json:"kind"`
		Payload struct {
			ProfileID string `json:"profile_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(createOut), &createView); err != nil {
		t.Fatalf("parse create output: %v\nraw=%s", err, createOut)
	}
	if createView.Kind != "permission_profile_detail" || createView.Payload.ProfileID != "owner-ops" {
		t.Fatalf("unexpected create payload: %#v", createView)
	}

	listOut := must("permission-profile", "list", "--json")
	var listView struct {
		Kind  string `json:"kind"`
		Items []struct {
			ProfileID string `json:"profile_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listView); err != nil {
		t.Fatalf("parse list output: %v\nraw=%s", err, listOut)
	}
	if listView.Kind != "permission_profile_list" || len(listView.Items) != 1 || listView.Items[0].ProfileID != "owner-ops" {
		t.Fatalf("unexpected profile list: %#v", listView)
	}

	agentViewOut := must("permissions", "view", "APP-1", "--actor", "agent:builder-1", "--action", "dispatch", "--json")
	var agentView struct {
		Kind    string `json:"kind"`
		Payload struct {
			Decisions []struct {
				Allowed               bool     `json:"allowed"`
				RequiresOwnerOverride bool     `json:"requires_owner_override"`
				OverrideApplied       bool     `json:"override_applied"`
				ReasonCodes           []string `json:"reason_codes"`
			} `json:"decisions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(agentViewOut), &agentView); err != nil {
		t.Fatalf("parse agent permissions view: %v\nraw=%s", err, agentViewOut)
	}
	if agentView.Kind != "permissions_effective_detail" || len(agentView.Payload.Decisions) != 1 {
		t.Fatalf("unexpected agent permissions view: %#v", agentView)
	}
	if agentView.Payload.Decisions[0].Allowed || !agentView.Payload.Decisions[0].RequiresOwnerOverride || agentView.Payload.Decisions[0].OverrideApplied {
		t.Fatalf("expected non-owner block with required override, got %#v", agentView.Payload.Decisions[0])
	}
	if !strings.Contains(strings.Join(agentView.Payload.Decisions[0].ReasonCodes, ","), "owner_override_required") {
		t.Fatalf("expected owner_override_required, got %#v", agentView.Payload.Decisions[0].ReasonCodes)
	}

	ownerViewOut := must("permissions", "view", "APP-1", "--actor", "human:owner", "--action", "dispatch", "--json")
	var ownerView struct {
		Payload struct {
			Decisions []struct {
				Allowed               bool `json:"allowed"`
				RequiresOwnerOverride bool `json:"requires_owner_override"`
				OverrideApplied       bool `json:"override_applied"`
			} `json:"decisions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(ownerViewOut), &ownerView); err != nil {
		t.Fatalf("parse owner permissions view: %v\nraw=%s", err, ownerViewOut)
	}
	if len(ownerView.Payload.Decisions) != 1 || !ownerView.Payload.Decisions[0].Allowed || !ownerView.Payload.Decisions[0].RequiresOwnerOverride || !ownerView.Payload.Decisions[0].OverrideApplied {
		t.Fatalf("expected owner override apply, got %#v", ownerView.Payload.Decisions)
	}

	if out, err := runCLI(t, "run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "agent:builder-1"); err == nil || !strings.Contains(out+err.Error(), "owner_override_required") {
		t.Fatalf("expected dispatch to be blocked for non-owner, err=%v out=%s", err, out)
	}

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	if dispatch.Payload.RunID == "" {
		t.Fatalf("expected dispatched run id")
	}
}

func TestPermissionProfileBindUnbindAndFlagGuardrails(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Scoped permissions", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")
	must("permission-profile", "create", "deny-dispatch", "--deny-action", "dispatch", "--actor", "human:owner")

	if out, err := runCLI(t, "permission-profile", "bind", "deny-dispatch", "--workspace", "--project", "APP", "--actor", "human:owner"); err == nil || !strings.Contains(out+err.Error(), "choose exactly one binding target") {
		t.Fatalf("expected binding target guardrail, err=%v out=%s", err, out)
	}

	must("permission-profile", "bind", "deny-dispatch", "--project", "APP", "--actor", "human:owner")
	blockedOut := must("permissions", "view", "APP-1", "--actor", "human:owner", "--action", "dispatch", "--json")
	var blocked struct {
		Payload struct {
			Decisions []struct {
				Allowed     bool     `json:"allowed"`
				ReasonCodes []string `json:"reason_codes"`
			} `json:"decisions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(blockedOut), &blocked); err != nil {
		t.Fatalf("parse blocked permissions view: %v\nraw=%s", err, blockedOut)
	}
	if len(blocked.Payload.Decisions) != 1 || blocked.Payload.Decisions[0].Allowed || !strings.Contains(strings.Join(blocked.Payload.Decisions[0].ReasonCodes, ","), "permission_action_denied") {
		t.Fatalf("expected bound deny profile to block dispatch, got %#v", blocked.Payload.Decisions)
	}

	must("permission-profile", "unbind", "deny-dispatch", "--project", "APP", "--actor", "human:owner")
	allowedOut := must("permissions", "view", "APP-1", "--actor", "human:owner", "--action", "dispatch", "--json")
	var allowed struct {
		Payload struct {
			Decisions []struct {
				Allowed bool `json:"allowed"`
			} `json:"decisions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(allowedOut), &allowed); err != nil {
		t.Fatalf("parse allowed permissions view: %v\nraw=%s", err, allowedOut)
	}
	if len(allowed.Payload.Decisions) != 1 || !allowed.Payload.Decisions[0].Allowed {
		t.Fatalf("expected unbound profile to restore dispatch access, got %#v", allowed.Payload.Decisions)
	}
}

func TestPermissionsViewReflectsCollaboratorLifecycleDenials(t *testing.T) {
	withTempWorkspace(t)
	gitRunCLI(t, "init", "-b", "main")
	gitRunCLI(t, "config", "user.email", "atlas@example.com")
	gitRunCLI(t, "config", "user.name", "Atlas")
	writeGitFile(t, "README.md", "# atlas\n")
	gitRunCLI(t, "add", "README.md")
	gitRunCLI(t, "commit", "-m", "init")

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Lifecycle gate", "--type", "task", "--actor", "human:owner")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")
	must("collaborator", "add", "builder-1", "--name", "Builder One", "--actor-map", "agent:builder-1", "--actor", "human:owner")
	must("collaborator", "trust", "builder-1", "--actor", "human:owner")
	must("membership", "bind", "builder-1", "--scope-kind", "project", "--scope-id", "APP", "--role", "contributor", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")

	dispatchOut := must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner", "--json")
	var dispatch struct {
		Payload struct {
			RunID string `json:"run_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(dispatchOut), &dispatch); err != nil {
		t.Fatalf("parse dispatch output: %v\nraw=%s", err, dispatchOut)
	}
	if dispatch.Payload.RunID == "" {
		t.Fatalf("expected dispatched run id")
	}

	must("run", "start", dispatch.Payload.RunID, "--actor", "human:owner")
	workspace, err := openWorkspace()
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	defer workspace.close()
	if _, err := workspace.actions.SetCollaboratorTrust(context.Background(), "builder-1", false, contracts.Actor("human:owner"), "revoke trust"); err != nil {
		t.Fatalf("revoke collaborator trust: %v", err)
	}

	blockedOut := must("permissions", "view", "run:"+dispatch.Payload.RunID, "--actor", "agent:builder-1", "--action", "run_complete", "--json")
	var blocked struct {
		Payload struct {
			Decisions []struct {
				Allowed     bool     `json:"allowed"`
				ReasonCodes []string `json:"reason_codes"`
			} `json:"decisions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(blockedOut), &blocked); err != nil {
		t.Fatalf("parse blocked permissions view: %v\nraw=%s", err, blockedOut)
	}
	if len(blocked.Payload.Decisions) != 1 || blocked.Payload.Decisions[0].Allowed || !strings.Contains(strings.Join(blocked.Payload.Decisions[0].ReasonCodes, ","), "collaborator_untrusted") {
		t.Fatalf("expected collaborator_untrusted denial, got %#v", blocked.Payload.Decisions)
	}

	if out, err := runCLI(t, "run", "complete", dispatch.Payload.RunID, "--summary", "done", "--actor", "agent:builder-1"); err == nil || !strings.Contains(out+err.Error(), "collaborator_untrusted") {
		t.Fatalf("expected run complete denial for untrusted collaborator, err=%v out=%s", err, out)
	}
}
