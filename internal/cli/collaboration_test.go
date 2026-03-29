package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRemoteSyncAndBundleCommands(t *testing.T) {
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
	must("ticket", "create", "--project", "APP", "--title", "Sync me", "--type", "task", "--actor", "human:owner")

	remoteDir := filepath.Join(t.TempDir(), "path-remote")
	addedOut := must("remote", "add", "origin", "--kind", "path", "--location", remoteDir, "--default-action", "push", "--actor", "human:owner", "--json")
	var added struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Remote struct {
				RemoteID      string `json:"remote_id"`
				DefaultAction string `json:"default_action"`
				Enabled       bool   `json:"enabled"`
			} `json:"remote"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(addedOut), &added); err != nil {
		t.Fatalf("parse remote add: %v\nraw=%s", err, addedOut)
	}
	if added.FormatVersion != jsonFormatVersion || added.Kind != "remote_detail" || added.Payload.Remote.RemoteID != "origin" || added.Payload.Remote.DefaultAction != "push" || !added.Payload.Remote.Enabled {
		t.Fatalf("unexpected remote add payload: %#v", added)
	}

	listOut := must("remote", "list", "--json")
	var listed struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Items         []struct {
			RemoteID string `json:"remote_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse remote list: %v\nraw=%s", err, listOut)
	}
	if listed.Kind != "remote_list" || len(listed.Items) != 1 || listed.Items[0].RemoteID != "origin" {
		t.Fatalf("unexpected remote list payload: %#v", listed)
	}

	editedOut := must("remote", "edit", "origin", "--default-action", "pull", "--enabled=false", "--actor", "human:owner", "--json")
	var edited struct {
		Payload struct {
			Remote struct {
				DefaultAction string `json:"default_action"`
				Enabled       bool   `json:"enabled"`
			} `json:"remote"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(editedOut), &edited); err != nil {
		t.Fatalf("parse remote edit: %v\nraw=%s", err, editedOut)
	}
	if edited.Payload.Remote.DefaultAction != "pull" || edited.Payload.Remote.Enabled {
		t.Fatalf("expected edited remote to be disabled pull, got %#v", edited)
	}
	must("remote", "edit", "origin", "--default-action", "push", "--enabled=true", "--actor", "human:owner")

	createOut := must("bundle", "create", "--actor", "human:owner", "--json")
	var created struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			Job struct {
				JobID     string `json:"job_id"`
				BundleRef string `json:"bundle_ref"`
			} `json:"job"`
			Publication struct {
				BundleID    string `json:"bundle_id"`
				WorkspaceID string `json:"workspace_id"`
			} `json:"publication"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("parse bundle create: %v\nraw=%s", err, createOut)
	}
	if created.Kind != "bundle_create_result" || created.Payload.Job.JobID == "" || created.Payload.Job.BundleRef == "" || created.Payload.Publication.BundleID == "" || created.Payload.Publication.WorkspaceID == "" {
		t.Fatalf("unexpected bundle create payload: %#v", created)
	}

	verifyOut := must("bundle", "verify", created.Payload.Job.BundleRef, "--actor", "human:owner", "--json")
	var verified struct {
		Kind    string `json:"kind"`
		Payload struct {
			Verified bool `json:"verified"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(verifyOut), &verified); err != nil {
		t.Fatalf("parse bundle verify: %v\nraw=%s", err, verifyOut)
	}
	if verified.Kind != "bundle_verify_result" || !verified.Payload.Verified {
		t.Fatalf("unexpected bundle verify payload: %#v", verified)
	}

	syncPushOut := must("sync", "push", "--remote", "origin", "--actor", "human:owner", "--json")
	var pushed struct {
		Kind    string `json:"kind"`
		Payload struct {
			Job struct {
				JobID string `json:"job_id"`
				Mode  string `json:"mode"`
			} `json:"job"`
			Remote struct {
				RemoteID string `json:"remote_id"`
			} `json:"remote"`
			Publication struct {
				BundleID string `json:"bundle_id"`
			} `json:"publication"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(syncPushOut), &pushed); err != nil {
		t.Fatalf("parse sync push: %v\nraw=%s", err, syncPushOut)
	}
	if pushed.Kind != "sync_job_detail" || pushed.Payload.Job.Mode != "push" || pushed.Payload.Remote.RemoteID != "origin" || pushed.Payload.Publication.BundleID == "" {
		t.Fatalf("unexpected sync push payload: %#v", pushed)
	}

	statusOut := must("sync", "status", "--remote", "origin", "--json")
	var status struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Payload       struct {
			MigrationComplete bool `json:"migration_complete"`
			Migration         struct {
				State string `json:"state"`
			} `json:"migration"`
			Remotes []struct {
				Publications []struct {
					BundleID string `json:"bundle_id"`
				} `json:"publications"`
			} `json:"remotes"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(statusOut), &status); err != nil {
		t.Fatalf("parse sync status: %v\nraw=%s", err, statusOut)
	}
	if status.FormatVersion != jsonFormatVersion || status.Kind != "sync_status" || !status.Payload.MigrationComplete || status.Payload.Migration.State != "stamped" || len(status.Payload.Remotes) != 1 || len(status.Payload.Remotes[0].Publications) != 1 {
		t.Fatalf("unexpected sync status payload: %#v", status)
	}

	jobsOut := must("sync", "jobs", "--remote", "origin", "--json")
	var jobs struct {
		Kind  string `json:"kind"`
		Items []struct {
			JobID string `json:"job_id"`
			Mode  string `json:"mode"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(jobsOut), &jobs); err != nil {
		t.Fatalf("parse sync jobs: %v\nraw=%s", err, jobsOut)
	}
	if jobs.Kind != "sync_job_list" || len(jobs.Items) == 0 {
		t.Fatalf("expected sync jobs, got %#v", jobs)
	}

	jobViewOut := must("sync", "view", pushed.Payload.Job.JobID, "--json")
	var jobView struct {
		Kind    string `json:"kind"`
		Payload struct {
			Job struct {
				JobID string `json:"job_id"`
			} `json:"job"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(jobViewOut), &jobView); err != nil {
		t.Fatalf("parse sync job view: %v\nraw=%s", err, jobViewOut)
	}
	if jobView.Kind != "sync_job_detail" || jobView.Payload.Job.JobID != pushed.Payload.Job.JobID {
		t.Fatalf("unexpected sync job view payload: %#v", jobView)
	}

	bundleListOut := must("bundle", "list", "--json")
	var bundleList struct {
		Kind  string `json:"kind"`
		Items []struct {
			JobID string `json:"job_id"`
			Mode  string `json:"mode"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(bundleListOut), &bundleList); err != nil {
		t.Fatalf("parse bundle list: %v\nraw=%s", err, bundleListOut)
	}
	if bundleList.Kind != "bundle_list" || len(bundleList.Items) == 0 {
		t.Fatalf("expected bundle jobs, got %#v", bundleList)
	}

	bundleViewOut := must("bundle", "view", created.Payload.Job.JobID, "--json")
	var bundleView struct {
		Kind    string `json:"kind"`
		Payload struct {
			Job struct {
				JobID string `json:"job_id"`
			} `json:"job"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(bundleViewOut), &bundleView); err != nil {
		t.Fatalf("parse bundle view: %v\nraw=%s", err, bundleViewOut)
	}
	if bundleView.Kind != "bundle_detail" || bundleView.Payload.Job.JobID != created.Payload.Job.JobID {
		t.Fatalf("unexpected bundle view payload: %#v", bundleView)
	}

	removeOut := must("remote", "remove", "origin", "--actor", "human:owner", "--json")
	var removed struct {
		Kind    string `json:"kind"`
		Payload struct {
			RemoteID string `json:"remote_id"`
			Removed  bool   `json:"removed"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(removeOut), &removed); err != nil {
		t.Fatalf("parse remote remove: %v\nraw=%s", err, removeOut)
	}
	if removed.Kind != "remote_detail" || removed.Payload.RemoteID != "origin" || !removed.Payload.Removed {
		t.Fatalf("unexpected remote remove payload: %#v", removed)
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

func TestConflictCommands(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	remoteDir := filepath.Join(t.TempDir(), "path-remote")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	runAt := func(dir string, args ...string) (string, error) {
		t.Helper()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir %s failed: %v", dir, err)
		}
		return runCLI(t, args...)
	}
	mustAt := func(dir string, args ...string) string {
		t.Helper()
		out, err := runAt(dir, args...)
		if err != nil {
			t.Fatalf("command failed in %s %v: %v\noutput=%s", dir, args, err, out)
		}
		return out
	}

	mustAt(sourceDir, "init")
	mustAt(sourceDir, "project", "create", "APP", "App Project")
	mustAt(sourceDir, "ticket", "create", "--project", "APP", "--title", "Remote title", "--type", "task", "--actor", "human:owner")
	mustAt(sourceDir, "ticket", "create", "--project", "APP", "--title", "Remote-only", "--type", "task", "--actor", "human:owner")
	mustAt(sourceDir, "remote", "add", "origin", "--kind", "path", "--location", remoteDir, "--default-action", "push", "--actor", "human:owner")
	pushOut := mustAt(sourceDir, "sync", "push", "--remote", "origin", "--actor", "human:owner", "--json")
	var pushed struct {
		Payload struct {
			Publication struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"publication"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(pushOut), &pushed); err != nil {
		t.Fatalf("parse source push: %v\nraw=%s", err, pushOut)
	}
	if pushed.Payload.Publication.WorkspaceID == "" {
		t.Fatalf("expected source workspace id, got %#v", pushed)
	}

	mustAt(targetDir, "init")
	mustAt(targetDir, "project", "create", "APP", "App Project")
	mustAt(targetDir, "ticket", "create", "--project", "APP", "--title", "Local title", "--type", "task", "--actor", "human:owner")
	mustAt(targetDir, "remote", "add", "origin", "--kind", "path", "--location", remoteDir, "--default-action", "pull", "--actor", "human:owner")
	if out, err := runAt(targetDir, "sync", "pull", "--remote", "origin", "--workspace", pushed.Payload.Publication.WorkspaceID, "--actor", "human:owner", "--json"); err == nil {
		t.Fatalf("expected sync pull conflict, got success output=%s", out)
	}

	conflictListOut := mustAt(targetDir, "conflict", "list", "--json")
	var listed struct {
		Kind  string `json:"kind"`
		Items []struct {
			ConflictID string `json:"conflict_id"`
			EntityKind string `json:"entity_kind"`
			Status     string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(conflictListOut), &listed); err != nil {
		t.Fatalf("parse conflict list: %v\nraw=%s", err, conflictListOut)
	}
	if listed.Kind != "conflict_list" || len(listed.Items) == 0 {
		t.Fatalf("unexpected conflict list payload: %#v", listed)
	}
	ticketConflictID := ""
	for _, item := range listed.Items {
		if item.EntityKind == "ticket" && item.Status == "open" {
			ticketConflictID = item.ConflictID
			break
		}
	}
	if ticketConflictID == "" {
		t.Fatalf("expected open ticket conflict in %#v", listed.Items)
	}

	conflictViewOut := mustAt(targetDir, "conflict", "view", ticketConflictID, "--json")
	var viewed struct {
		Kind    string `json:"kind"`
		Payload struct {
			Conflict struct {
				ConflictID string `json:"conflict_id"`
			} `json:"conflict"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(conflictViewOut), &viewed); err != nil {
		t.Fatalf("parse conflict view: %v\nraw=%s", err, conflictViewOut)
	}
	if viewed.Kind != "conflict_detail" || viewed.Payload.Conflict.ConflictID != ticketConflictID {
		t.Fatalf("unexpected conflict view payload: %#v", viewed)
	}

	resolveOut := mustAt(targetDir, "conflict", "resolve", ticketConflictID, "--resolution", "use_remote", "--actor", "human:owner", "--json")
	var resolved struct {
		Kind    string `json:"kind"`
		Payload struct {
			Conflict struct {
				Status     string `json:"status"`
				Resolution string `json:"resolution"`
			} `json:"conflict"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(resolveOut), &resolved); err != nil {
		t.Fatalf("parse conflict resolve: %v\nraw=%s", err, resolveOut)
	}
	if resolved.Kind != "conflict_resolve_result" || resolved.Payload.Conflict.Status != "resolved" || resolved.Payload.Conflict.Resolution != "use_remote" {
		t.Fatalf("unexpected resolve payload: %#v", resolved)
	}

	ticketOut := mustAt(targetDir, "ticket", "view", "APP-1", "--json")
	var ticket struct {
		Ticket struct {
			Title string `json:"title"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(ticketOut), &ticket); err != nil {
		t.Fatalf("parse ticket view after resolve: %v\nraw=%s", err, ticketOut)
	}
	if ticket.Ticket.Title != "Remote title" {
		t.Fatalf("expected remote ticket title after resolve, got %#v", ticket)
	}

	ticketTwoOut := mustAt(targetDir, "ticket", "view", "APP-2", "--json")
	var ticketTwo struct {
		Ticket struct {
			Title string `json:"title"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(ticketTwoOut), &ticketTwo); err != nil {
		t.Fatalf("parse remote-only ticket: %v\nraw=%s", err, ticketTwoOut)
	}
	if ticketTwo.Ticket.Title != "Remote-only" {
		t.Fatalf("expected remote-only ticket to apply, got %#v", ticketTwo)
	}
}

func TestProjectCodeownersAndRulesCommands(t *testing.T) {
	workspaceDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(workspaceDir); err != nil {
		t.Fatalf("chdir workspace failed: %v", err)
	}

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
	must("collaborator", "add", "rev-1", "--name", "Rev One", "--actor-map", "agent:reviewer-1", "--provider-handle", "github:rev-one", "--actor", "human:owner")
	must("collaborator", "trust", "rev-1", "--actor", "human:owner")
	must("membership", "bind", "rev-1", "--scope-kind", "project", "--scope-id", "APP", "--role", "reviewer", "--actor", "human:owner")
	gitInit := exec.Command("git", "init")
	gitInit.Dir = workspaceDir
	if raw, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, raw)
	}

	renderOut := must("project", "codeowners", "render", "APP", "--json")
	var rendered struct {
		Kind    string `json:"kind"`
		Payload struct {
			Content string `json:"content"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(renderOut), &rendered); err != nil {
		t.Fatalf("parse codeowners render: %v\nraw=%s", err, renderOut)
	}
	if rendered.Kind != "codeowners_preview" || !strings.Contains(rendered.Payload.Content, "* @rev-one") {
		t.Fatalf("unexpected codeowners preview: %#v", rendered)
	}

	writeOut := must("project", "codeowners", "write", "APP", "--actor", "human:owner", "--reason", "write codeowners", "--json")
	var written struct {
		Payload struct {
			Path string `json:"path"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(writeOut), &written); err != nil {
		t.Fatalf("parse codeowners write: %v\nraw=%s", err, writeOut)
	}
	raw, err := os.ReadFile(filepath.Join(workspaceDir, "CODEOWNERS"))
	if err != nil {
		t.Fatalf("read CODEOWNERS: %v", err)
	}
	expectedPath := filepath.Join(workspaceDir, "CODEOWNERS")
	expectedPath, err = filepath.EvalSymlinks(expectedPath)
	if err != nil {
		t.Fatalf("resolve CODEOWNERS path: %v", err)
	}
	if written.Payload.Path != expectedPath || !strings.Contains(string(raw), "@rev-one") {
		t.Fatalf("unexpected CODEOWNERS write payload=%#v raw=%s", written, string(raw))
	}

	rulesOut := must("project", "rules", "render", "APP", "--json")
	var rules struct {
		Kind    string `json:"kind"`
		Payload struct {
			Rules []struct {
				Name      string   `json:"name"`
				Reviewers []string `json:"reviewers"`
			} `json:"rules"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(rulesOut), &rules); err != nil {
		t.Fatalf("parse provider rules preview: %v\nraw=%s", err, rulesOut)
	}
	if rules.Kind != "provider_rules_preview" || len(rules.Payload.Rules) == 0 || !slices.Contains(rules.Payload.Rules[0].Reviewers, "@rev-one") {
		t.Fatalf("unexpected provider rules preview: %#v", rules)
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
