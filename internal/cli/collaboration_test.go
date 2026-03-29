package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestV16ReadStubsUseContractShape(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	statusOut, err := runCLI(t, "sync", "status", "--json")
	if err != nil {
		t.Fatalf("sync status failed: %v", err)
	}
	var status struct {
		FormatVersion string         `json:"format_version"`
		Kind          string         `json:"kind"`
		Payload       map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("parse sync status json: %v\nraw=%s", err, statusOut)
	}
	if status.FormatVersion != jsonFormatVersion || status.Kind != "sync_status" || status.Payload == nil {
		t.Fatalf("unexpected sync status stub payload: %#v", status)
	}

	remoteOut, err := runCLI(t, "remote", "view", "origin", "--json")
	if err != nil {
		t.Fatalf("remote view failed: %v", err)
	}
	var remote struct {
		FormatVersion string         `json:"format_version"`
		Kind          string         `json:"kind"`
		Payload       map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(remoteOut), &remote); err != nil {
		t.Fatalf("parse remote detail json: %v\nraw=%s", err, remoteOut)
	}
	if remote.FormatVersion != jsonFormatVersion || remote.Kind != "remote_detail" || remote.Payload == nil {
		t.Fatalf("unexpected remote detail stub payload: %#v", remote)
	}
}

func TestV16MutationStubsReturnJSONErrorExit(t *testing.T) {
	withTempWorkspace(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute([]string{"remote", "add", "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected remote add stub to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout for json error path, got %s", stdout.String())
	}
	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
			Exit int    `json:"exit"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("parse json error envelope: %v\nraw=%s", err, stderr.String())
	}
	if envelope.OK || envelope.Error.Exit == 0 {
		t.Fatalf("unexpected json error envelope: %#v", envelope)
	}
}

func TestCollaboratorMembershipAndMentionsCommands(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\noutput=%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Mention me", "--type", "task", "--actor", "human:owner")
	must("collaborator", "add", "alana", "--name", "Alana", "--actor-map", "agent:reviewer-1", "--actor", "human:owner")
	must("membership", "bind", "alana", "--scope-kind", "project", "--scope-id", "APP", "--role", "reviewer", "--profile", "audit-ops", "--actor", "human:owner")
	must("ticket", "comment", "APP-1", "--body", "ping @alana mail alana@example.com `@alana` /tmp/@alana https://x.example/@alana \\@alana", "--actor", "human:owner")

	mentionsOut := must("mentions", "list", "--collaborator", "alana", "--json")
	var mentions struct {
		FormatVersion string             `json:"format_version"`
		Kind          string             `json:"kind"`
		Items         []contractsMention `json:"items"`
	}
	if err := json.Unmarshal([]byte(mentionsOut), &mentions); err != nil {
		t.Fatalf("parse mentions list: %v\nraw=%s", err, mentionsOut)
	}
	if mentions.Kind != "mentions_list" || len(mentions.Items) != 1 {
		t.Fatalf("expected one canonical mention, got %#v", mentions)
	}

	mentionOut := must("mentions", "view", mentions.Items[0].MentionUID, "--json")
	var mentionDetail struct {
		Payload struct {
			Mention struct {
				MentionUID     string `json:"mention_uid"`
				CollaboratorID string `json:"collaborator_id"`
				SourceKind     string `json:"source_kind"`
			} `json:"mention"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(mentionOut), &mentionDetail); err != nil {
		t.Fatalf("parse mention detail: %v\nraw=%s", err, mentionOut)
	}
	if mentionDetail.Payload.Mention.CollaboratorID != "alana" || mentionDetail.Payload.Mention.SourceKind != "ticket_comment" {
		t.Fatalf("unexpected mention detail: %#v", mentionDetail)
	}

	collaboratorOut := must("collaborator", "view", "alana", "--json")
	var collaboratorDetail struct {
		Payload struct {
			Collaborator struct {
				CollaboratorID string `json:"collaborator_id"`
			} `json:"collaborator"`
			Memberships []struct {
				MembershipUID string `json:"membership_uid"`
				Status        string `json:"status"`
			} `json:"memberships"`
			Mentions []struct {
				MentionUID string `json:"mention_uid"`
			} `json:"mentions"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(collaboratorOut), &collaboratorDetail); err != nil {
		t.Fatalf("parse collaborator detail: %v\nraw=%s", err, collaboratorOut)
	}
	if collaboratorDetail.Payload.Collaborator.CollaboratorID != "alana" || len(collaboratorDetail.Payload.Memberships) != 1 || len(collaboratorDetail.Payload.Mentions) != 1 {
		t.Fatalf("unexpected collaborator detail payload: %#v", collaboratorDetail)
	}

	membershipUID := collaboratorDetail.Payload.Memberships[0].MembershipUID
	membershipOut := must("membership", "unbind", membershipUID, "--actor", "human:owner", "--json")
	var membershipResult struct {
		Items []struct {
			MembershipUID string `json:"membership_uid"`
			Status        string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(membershipOut), &membershipResult); err != nil {
		t.Fatalf("parse membership result: %v\nraw=%s", err, membershipOut)
	}
	if len(membershipResult.Items) != 1 || membershipResult.Items[0].Status != string(contractsMembershipStatusUnbound()) {
		t.Fatalf("expected unbound membership, got %#v", membershipResult)
	}
}

func TestCollaboratorFilteredInboxAndApprovals(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\noutput=%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")
	must("collaborator", "add", "rev-1", "--name", "Rev One", "--actor-map", "agent:reviewer-1", "--actor", "human:owner")
	must("membership", "bind", "rev-1", "--scope-kind", "project", "--scope-id", "APP", "--role", "reviewer", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Needs review", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("run", "dispatch", "APP-1", "--agent", "builder-1", "--actor", "human:owner")

	runsOut := must("run", "list", "--json")
	var runs struct {
		Items []struct {
			RunID string `json:"run_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(runsOut), &runs); err != nil {
		t.Fatalf("parse run list: %v\nraw=%s", err, runsOut)
	}
	if len(runs.Items) != 1 {
		t.Fatalf("expected one run, got %#v", runs)
	}
	runID := runs.Items[0].RunID

	must("run", "start", runID, "--actor", "human:owner")
	must("run", "handoff", runID, "--next-actor", "agent:reviewer-1", "--next-gate", "review", "--open-question", "check with @rev-1", "--actor", "human:owner")

	approvalsOut := must("approvals", "--collaborator", "rev-1", "--json")
	var approvals struct {
		Items []struct {
			CollaboratorIDs []string `json:"collaborator_ids"`
			Gate            struct {
				GateID string `json:"gate_id"`
			} `json:"gate"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(approvalsOut), &approvals); err != nil {
		t.Fatalf("parse approvals: %v\nraw=%s", err, approvalsOut)
	}
	if len(approvals.Items) != 1 || approvals.Items[0].Gate.GateID == "" {
		t.Fatalf("expected one collaborator-routed approval, got %#v", approvals)
	}

	inboxOut := must("inbox", "--collaborator", "rev-1", "--json")
	var inbox struct {
		Items []struct {
			Kind string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(inboxOut), &inbox); err != nil {
		t.Fatalf("parse inbox: %v\nraw=%s", err, inboxOut)
	}
	if len(inbox.Items) < 2 {
		t.Fatalf("expected collaborator inbox to include routed work and mention, got %#v", inbox)
	}
}

func TestCollaboratorRoutingRespectsProjectMembership(t *testing.T) {
	withTempWorkspace(t)

	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("command failed %v: %v\noutput=%s", args, err, out)
		}
		return out
	}

	must("init")
	must("project", "create", "APP", "App Project")
	must("project", "create", "WEB", "Web Project")
	must("agent", "create", "builder-1", "--name", "Builder One", "--provider", "codex", "--capability", "go", "--actor", "human:owner")
	must("collaborator", "add", "rev-1", "--name", "Rev One", "--actor-map", "agent:reviewer-1", "--actor", "human:owner")
	must("membership", "bind", "rev-1", "--scope-kind", "project", "--scope-id", "APP", "--role", "reviewer", "--actor", "human:owner")

	for _, projectKey := range []string{"APP", "WEB"} {
		must("ticket", "create", "--project", projectKey, "--title", projectKey+" review", "--type", "task", "--reviewer", "agent:reviewer-1", "--actor", "human:owner")
		ticketID := projectKey + "-1"
		must("ticket", "move", ticketID, "ready", "--actor", "human:owner")
		must("run", "dispatch", ticketID, "--agent", "builder-1", "--actor", "human:owner")
		runsOut := must("run", "list", "--ticket", ticketID, "--json")
		var runs struct {
			Items []struct {
				RunID string `json:"run_id"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(runsOut), &runs); err != nil {
			t.Fatalf("parse run list for %s: %v\nraw=%s", projectKey, err, runsOut)
		}
		if len(runs.Items) != 1 {
			t.Fatalf("expected one run for %s, got %#v", projectKey, runs)
		}
		runID := runs.Items[0].RunID
		must("run", "start", runID, "--actor", "human:owner")
		must("run", "handoff", runID, "--next-actor", "agent:reviewer-1", "--next-gate", "review", "--actor", "human:owner")
	}

	approvalsOut := must("approvals", "--collaborator", "rev-1", "--json")
	var approvals struct {
		Items []struct {
			Ticket struct {
				ID string `json:"id"`
			} `json:"ticket"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(approvalsOut), &approvals); err != nil {
		t.Fatalf("parse approvals: %v\nraw=%s", err, approvalsOut)
	}
	if len(approvals.Items) != 1 || approvals.Items[0].Ticket.ID != "APP-1" {
		t.Fatalf("expected only APP approval routing, got %#v", approvals)
	}

	inboxOut := must("inbox", "--collaborator", "rev-1", "--json")
	var inbox struct {
		Items []struct {
			TicketID string `json:"ticket_id"`
			Kind     string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(inboxOut), &inbox); err != nil {
		t.Fatalf("parse inbox: %v\nraw=%s", err, inboxOut)
	}
	for _, item := range inbox.Items {
		if item.Kind != "mention" && item.TicketID != "APP-1" {
			t.Fatalf("expected collaborator routing to stay on APP only, got %#v", inbox.Items)
		}
	}
}

func TestCollaboratorAddRejectsInvalidProviderHandle(t *testing.T) {
	withTempWorkspace(t)
	if _, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	out, err := runCLI(t, "collaborator", "add", "alana", "--provider-handle", "github", "--actor", "human:owner")
	if err == nil {
		t.Fatalf("expected invalid provider handle to fail, got output=%s", out)
	}
}

type contractsMention struct {
	MentionUID     string `json:"mention_uid"`
	CollaboratorID string `json:"collaborator_id"`
}

func contractsMembershipStatusUnbound() string {
	return "unbound"
}
