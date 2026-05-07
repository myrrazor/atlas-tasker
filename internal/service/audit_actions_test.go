package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestAuditReportPacketSignAndTamperDetection(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "owner"}, contracts.Actor("human:owner"), "audit signing key")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "owner", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust audit signer"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}

	reportView, err := actions.CreateAuditReport(ctx, "ticket:"+ticket.ID, contracts.Actor("human:owner"), "create release audit report")
	if err != nil {
		t.Fatalf("create audit report: %v", err)
	}
	report := reportView.Report
	if report.ScopeKind != contracts.AuditScopeTicket || report.ScopeID != ticket.ID || report.EventRange.FromEventID == 0 || report.PolicySnapshotHash == "" || report.TrustSnapshotHash == "" {
		t.Fatalf("audit report did not bind scope/ranges/snapshots: %#v", report)
	}
	if len(report.IncludedArtifactHashes) == 0 {
		t.Fatalf("audit report should hash at least one Atlas-owned artifact")
	}
	unsigned, err := actions.VerifyAuditArtifact(ctx, report.AuditReportID)
	if err != nil {
		t.Fatalf("verify unsigned report: %v", err)
	}
	if unsigned.Signature.State != contracts.VerificationMissingSignature {
		t.Fatalf("unsigned audit report should be missing_signature, got %#v", unsigned.Signature)
	}

	signedReport, err := actions.SignAuditReport(ctx, report.AuditReportID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign audit report")
	if err != nil {
		t.Fatalf("sign report: %v", err)
	}
	if signedReport.ArtifactKind != contracts.ArtifactKindAuditReport {
		t.Fatalf("unexpected report signature: %#v", signedReport)
	}
	reportVerify, err := actions.VerifyAuditArtifact(ctx, report.AuditReportID)
	if err != nil {
		t.Fatalf("verify signed report: %v", err)
	}
	if reportVerify.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("signed report should verify trusted, got %#v", reportVerify.Signature)
	}
	if reportVerify.Kind != "audit_report_verify_result" {
		t.Fatalf("report verification should have report-specific kind, got %s", reportVerify.Kind)
	}

	packetView, err := actions.ExportAuditPacket(ctx, report.AuditReportID, contracts.Actor("human:owner"), "export audit packet")
	if err != nil {
		t.Fatalf("export audit packet: %v", err)
	}
	projectEvents, err := actions.Events.StreamEvents(ctx, ticket.Project, 0)
	if err != nil {
		t.Fatalf("stream project events: %v", err)
	}
	foundExportEvent := false
	for _, event := range projectEvents {
		if event.Type == contracts.EventAuditReportExported {
			foundExportEvent = true
			break
		}
	}
	if !foundExportEvent {
		t.Fatalf("ticket-scoped packet export should be auditable in %s event stream", ticket.Project)
	}
	packet := packetView.Packet
	if packet.PacketHash == "" || packet.Report.AuditReportID != report.AuditReportID {
		t.Fatalf("bad audit packet: %#v", packet)
	}
	if len(packet.Report.SignatureEnvelopes) != 0 {
		t.Fatalf("audit packets should not embed nested report signatures: %#v", packet.Report.SignatureEnvelopes)
	}
	tamperedStored := packet
	tamperedStored.PacketID = "packet-tampered-store"
	tamperedStored.PacketHash = strings.Repeat("1", 64)
	if err := actions.AuditPackets.SaveAuditPacket(ctx, tamperedStored); err != nil {
		t.Fatalf("save tampered stored packet: %v", err)
	}
	if _, err := actions.SignAuditPacket(ctx, tamperedStored.PacketID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign tampered packet"); err == nil || !strings.Contains(err.Error(), "integrity must verify") {
		t.Fatalf("tampered stored packet should not be signable, got %v", err)
	}
	if _, err := actions.SignAuditPacket(ctx, packet.PacketID, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign audit packet"); err != nil {
		t.Fatalf("sign packet: %v", err)
	}
	if _, err := actions.VerifyAuditReportArtifact(ctx, packet.PacketID); err == nil || !strings.Contains(err.Error(), "expected audit report") {
		t.Fatalf("packet should not verify through report-only path, got %v", err)
	}
	if _, err := actions.VerifyAuditPacketArtifact(ctx, report.AuditReportID); err == nil || !strings.Contains(err.Error(), "expected audit packet") {
		t.Fatalf("report should not verify through packet-only path, got %v", err)
	}
	packetVerify, err := actions.VerifyAuditArtifact(ctx, packet.PacketID)
	if err != nil {
		t.Fatalf("verify packet: %v", err)
	}
	if packetVerify.Signature.State != contracts.VerificationTrustedValid {
		t.Fatalf("signed packet should verify trusted, got %#v", packetVerify.Signature)
	}
	if packetVerify.Kind != "audit_packet_verify_result" {
		t.Fatalf("packet verification should have packet-specific kind, got %s", packetVerify.Kind)
	}
	integrity, ok := packetVerify.Integrity.(AuditIntegrityView)
	if !ok || !integrity.Verified || integrity.PacketHash == "" || integrity.PacketHash != integrity.ExpectedPacketHash {
		t.Fatalf("packet integrity should verify: %#v", packetVerify.Integrity)
	}

	raw, err := os.ReadFile(packetView.Path)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	var tampered contracts.AuditPacket
	if err := json.Unmarshal(raw, &tampered); err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	tampered.PacketHash = strings.Repeat("0", 64)
	tamperedRaw, err := json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered packet: %v", err)
	}
	tamperedPath := packetView.Path + ".tampered.json"
	if err := os.WriteFile(tamperedPath, append(tamperedRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered packet: %v", err)
	}
	tamperedVerify, err := actions.VerifyAuditArtifact(ctx, tamperedPath)
	if err != nil {
		t.Fatalf("verify tampered packet: %v", err)
	}
	tamperedIntegrity, ok := tamperedVerify.Integrity.(AuditIntegrityView)
	if !ok || tamperedIntegrity.Verified || !strings.Contains(strings.Join(tamperedIntegrity.Errors, ","), "packet_hash_mismatch") {
		t.Fatalf("tampered packet should fail integrity: %#v", tamperedVerify.Integrity)
	}

	bodyTampered := packet
	bodyTampered.Report.Findings = append(bodyTampered.Report.Findings, contracts.AuditFinding{
		FindingID: "tampered-body",
		Severity:  contracts.AuditFindingCritical,
		Code:      "tampered",
		Message:   "body changed after packet hash",
	})
	bodyTamperedRaw, err := json.MarshalIndent(bodyTampered, "", "  ")
	if err != nil {
		t.Fatalf("encode body-tampered packet: %v", err)
	}
	bodyTamperedPath := packetView.Path + ".body-tampered.json"
	if err := os.WriteFile(bodyTamperedPath, append(bodyTamperedRaw, '\n'), 0o644); err != nil {
		t.Fatalf("write body-tampered packet: %v", err)
	}
	bodyTamperedVerify, err := actions.VerifyAuditArtifact(ctx, bodyTamperedPath)
	if err != nil {
		t.Fatalf("verify body-tampered packet: %v", err)
	}
	bodyTamperedIntegrity, ok := bodyTamperedVerify.Integrity.(AuditIntegrityView)
	if !ok || bodyTamperedIntegrity.Verified || !strings.Contains(strings.Join(bodyTamperedIntegrity.Errors, ","), "packet_hash_mismatch") {
		t.Fatalf("body-tampered packet should fail integrity: %#v", bodyTamperedVerify.Integrity)
	}

	if _, err := actions.RevokeKey(ctx, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "revoke audit signer"); err != nil {
		t.Fatalf("revoke audit signer: %v", err)
	}
	revokedVerify, err := actions.VerifyAuditArtifact(ctx, packet.PacketID)
	if err != nil {
		t.Fatalf("verify packet after revocation: %v", err)
	}
	if revokedVerify.Signature.State != contracts.VerificationValidRevokedKey {
		t.Fatalf("revoked packet signer should be surfaced, got %#v", revokedVerify.Signature)
	}
	if err := os.Remove(storage.PublicKeyFile(actions.Root, key.PublicKey.PublicKeyID)); err != nil {
		t.Fatalf("remove public key: %v", err)
	}
	unknownVerify, err := actions.VerifyAuditArtifact(ctx, packet.PacketID)
	if err != nil {
		t.Fatalf("verify packet after public key removal: %v", err)
	}
	if unknownVerify.Signature.State != contracts.VerificationValidUnknownKey {
		t.Fatalf("unknown packet signer should be surfaced, got %#v", unknownVerify.Signature)
	}
}

func TestAuditTicketScopeHashesRelatedArtifacts(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	now := actions.now()
	run := contracts.RunSnapshot{
		RunID:         "run-related",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		Status:        contracts.RunStatusCompleted,
		Kind:          contracts.RunKindWork,
		CreatedAt:     now,
		CompletedAt:   now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := actions.Evidence.SaveEvidence(ctx, contracts.EvidenceItem{
		EvidenceID:    "evidence-related",
		RunID:         run.RunID,
		TicketID:      ticket.ID,
		Type:          contracts.EvidenceTypeNote,
		Title:         "test proof",
		Body:          "go test passed",
		Actor:         contracts.Actor("human:owner"),
		CreatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save evidence: %v", err)
	}
	if err := actions.Handoffs.SaveHandoff(ctx, contracts.HandoffPacket{
		HandoffID:     "handoff-related",
		SourceRunID:   run.RunID,
		TicketID:      ticket.ID,
		Actor:         contracts.Actor("human:owner"),
		StatusSummary: "ready",
		GeneratedAt:   now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save handoff: %v", err)
	}
	if err := actions.Gates.SaveGate(ctx, contracts.GateSnapshot{
		GateID:        "gate-related",
		TicketID:      ticket.ID,
		RunID:         run.RunID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateApproved,
		CreatedBy:     contracts.Actor("human:owner"),
		DecidedBy:     contracts.Actor("human:owner"),
		CreatedAt:     now,
		DecidedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save gate: %v", err)
	}
	if err := actions.Changes.SaveChange(ctx, contracts.ChangeRef{
		ChangeID:      "change-related",
		Provider:      contracts.ChangeProviderGitHub,
		TicketID:      ticket.ID,
		RunID:         run.RunID,
		BranchName:    "run/related",
		Status:        contracts.ChangeStatusMerged,
		ChecksStatus:  contracts.CheckAggregatePassing,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save change: %v", err)
	}
	report, err := actions.CreateAuditReport(ctx, "ticket:"+ticket.ID, contracts.Actor("human:owner"), "hash related artifacts")
	if err != nil {
		t.Fatalf("create audit report: %v", err)
	}
	got := strings.Join(func() []string {
		ids := make([]string, 0, len(report.Report.IncludedArtifactHashes))
		for _, artifact := range report.Report.IncludedArtifactHashes {
			ids = append(ids, artifact.UID)
		}
		return ids
	}(), "\n")
	for _, want := range []string{"run-related", "evidence-related", "handoff-related", "gate-related", "change-related"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ticket audit hashes should include related %s artifact, got:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "file:.tracker/events/") {
		t.Fatalf("scoped audit should hash the event log backing its event range, got:\n%s", got)
	}
}

func TestAuditExportEventCarriesScopeMetadata(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	run := contracts.RunSnapshot{
		RunID:         "run-audit-export",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		Status:        contracts.RunStatusCompleted,
		Kind:          contracts.RunKindWork,
		CreatedAt:     actions.now(),
		CompletedAt:   actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	report, err := actions.CreateAuditReport(ctx, "run:"+run.RunID, contracts.Actor("human:owner"), "run audit")
	if err != nil {
		t.Fatalf("create run report: %v", err)
	}
	if _, err := actions.ExportAuditPacket(ctx, report.Report.AuditReportID, contracts.Actor("human:owner"), "export run audit packet"); err != nil {
		t.Fatalf("export run audit packet: %v", err)
	}
	events, err := actions.Events.StreamEvents(ctx, ticket.Project, 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	for _, event := range events {
		if event.Type != contracts.EventAuditReportExported {
			continue
		}
		if !auditEventMatchesScope(event, auditScope{Kind: contracts.AuditScopeRun, ID: run.RunID}) {
			t.Fatalf("run-scoped export event should be rediscoverable: %#v", event.Payload)
		}
		raw, _ := json.Marshal(event.Payload)
		for _, want := range []string{`"scope_kind":"run"`, `"scope_id":"run-audit-export"`, `"run_id":"run-audit-export"`, `"ticket_id":"` + ticket.ID + `"`} {
			if !strings.Contains(string(raw), want) {
				t.Fatalf("export event payload missing %s: %s", want, raw)
			}
		}
		return
	}
	t.Fatalf("missing audit export event")
}

func TestAuditTicketScopeUsesExactIDs(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	now := actions.now()
	near := contracts.TicketSnapshot{
		ID:            "APP-10",
		Project:       ticket.Project,
		Title:         "Nearby id",
		Type:          contracts.TicketTypeTask,
		Status:        contracts.StatusReady,
		Priority:      contracts.PriorityMedium,
		CreatedAt:     now,
		UpdatedAt:     now,
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Tickets.CreateTicket(ctx, near); err != nil {
		t.Fatalf("create nearby ticket: %v", err)
	}
	if err := actions.AppendAndProject(ctx, contracts.Event{EventID: 2, Timestamp: now.Add(time.Second), Actor: contracts.Actor("agent:builder-2"), Type: contracts.EventTicketCreated, Project: near.Project, TicketID: near.ID, Payload: near, SchemaVersion: contracts.CurrentSchemaVersion}); err != nil {
		t.Fatalf("append nearby event: %v", err)
	}
	report, err := actions.CreateAuditReport(ctx, "ticket:"+ticket.ID, contracts.Actor("human:owner"), "exact ticket audit")
	if err != nil {
		t.Fatalf("create audit report: %v", err)
	}
	if report.Report.EventRange.ToEventID != 1 {
		t.Fatalf("APP-1 audit should not include APP-10 event: %#v", report.Report.EventRange)
	}
	for _, artifact := range report.Report.IncludedArtifactHashes {
		if strings.Contains(artifact.UID, near.ID) {
			t.Fatalf("APP-1 audit should not include APP-10 artifact: %#v", artifact)
		}
	}
}

func TestAuditPolicyExplainRequiresEventUID(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	if _, err := actions.ExplainAuditPolicy(ctx, "1"); err == nil || !strings.Contains(err.Error(), "event_uid is required") {
		t.Fatalf("bare numeric event id should be rejected, got %v", err)
	}
	events, err := actions.Events.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	if len(events) == 0 || events[0].EventUID == "" {
		t.Fatalf("expected normalized event uid: %#v", events)
	}
	view, err := actions.ExplainAuditPolicy(ctx, events[0].EventUID)
	if err != nil {
		t.Fatalf("explain by event uid: %v", err)
	}
	if view.Event == nil || view.Event.EventUID != events[0].EventUID || view.Target != events[0].EventUID {
		t.Fatalf("wrong explanation target: %#v", view)
	}

	if err := actions.GovernancePolicies.SaveGovernancePolicy(ctx, contracts.GovernancePolicy{
		PolicyID:         "ticket-complete-review",
		Name:             "Ticket complete review",
		ScopeKind:        contracts.PolicyScopeProject,
		ScopeID:          "APP",
		ProtectedActions: []contracts.ProtectedAction{contracts.ProtectedActionTicketComplete},
		QuorumRules: []contracts.QuorumRule{{
			RuleID:        "needs-reviewer",
			ActionKind:    contracts.ProtectedActionTicketComplete,
			RequiredCount: 1,
		}},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("save governance policy: %v", err)
	}
	if err := actions.AppendAndProject(ctx, contracts.Event{
		EventID:       2,
		Timestamp:     actions.now().Add(time.Minute),
		Actor:         contracts.Actor("agent:builder-1"),
		Reason:        "complete ticket",
		Type:          contracts.EventTicketMoved,
		Project:       "APP",
		TicketID:      ticket.ID,
		Payload:       map[string]any{"from": contracts.StatusInReview, "to": contracts.StatusDone, "ticket": ticket},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}); err != nil {
		t.Fatalf("append completion event: %v", err)
	}
	events, err = actions.Events.StreamEvents(ctx, "APP", 0)
	if err != nil {
		t.Fatalf("stream updated events: %v", err)
	}
	explained, err := actions.ExplainAuditPolicy(ctx, events[len(events)-1].EventUID)
	if err != nil {
		t.Fatalf("explain completion event: %v", err)
	}
	if explained.Governance == nil || explained.Inputs["action"] != string(contracts.ProtectedActionTicketComplete) || explained.Inputs["actor"] != "agent:builder-1" {
		t.Fatalf("policy explanation should surface governance inputs: %#v", explained)
	}
	reasons := strings.Join(explained.ReasonCodes, ",")
	if !strings.Contains(reasons, "governance_denied") || !strings.Contains(strings.Join(explained.Governance.ReasonCodes, ","), "quorum_unsatisfied") {
		t.Fatalf("policy explanation should distinguish denied governance: %#v", explained)
	}
}

func TestAuditSnapshotHashesUseAtlasCanonicalization(t *testing.T) {
	snapshot := map[string]any{
		"policies": []contracts.GovernancePolicy{{
			PolicyID:      "policy-1",
			Name:          "Canonical policy",
			Description:   "line\r\nbreak",
			ScopeKind:     contracts.PolicyScopeWorkspace,
			CreatedAt:     time.Date(2026, 5, 6, 12, 0, 0, 0, time.FixedZone("offset", -4*60*60)),
			UpdatedAt:     time.Date(2026, 5, 6, 12, 0, 0, 0, time.FixedZone("offset", -4*60*60)),
			SchemaVersion: contracts.CurrentSchemaVersion,
		}},
		"packs": []contracts.PolicyPack{},
	}
	raw, err := contracts.CanonicalizeAtlasV1(snapshot)
	if err != nil {
		t.Fatalf("canonicalize snapshot: %v", err)
	}
	if !strings.Contains(string(raw), `line\nbreak`) || strings.Contains(string(raw), "\r") {
		t.Fatalf("snapshot bytes should be c14n-normalized, got %s", raw)
	}
	sum := sha256.Sum256(raw)
	want := hex.EncodeToString(sum[:])
	got, err := canonicalSnapshotHash(snapshot)
	if err != nil {
		t.Fatalf("hash snapshot: %v", err)
	}
	if got != want {
		t.Fatalf("snapshot hash should be over atlas-c14n-v1 bytes: got %s want %s", got, want)
	}
}

func TestAuditArtifactPathMustStayInsideWorkspace(t *testing.T) {
	ctx, actions, _ := newGovernanceHarness(t)
	outside := actions.Root + "-outside.json"
	if err := os.WriteFile(outside, []byte(`{"audit_report_id":"audit_outside"}`), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	if _, err := actions.VerifyAuditArtifact(ctx, outside); err == nil || !strings.Contains(err.Error(), "inside the workspace") {
		t.Fatalf("outside audit path should be rejected, got %v", err)
	}
}

func TestHandoffEvidenceAndApprovalSignaturesUseSharedTrust(t *testing.T) {
	ctx, actions, ticket := newGovernanceHarness(t)
	key, err := actions.GenerateKey(ctx, KeyGenerateOptions{Scope: contracts.KeyScopeCollaborator, OwnerID: "owner"}, contracts.Actor("human:owner"), "shared signer")
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if _, err := actions.BindTrust(ctx, "owner", key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "trust shared signer"); err != nil {
		t.Fatalf("bind trust: %v", err)
	}
	run := contracts.RunSnapshot{
		RunID:         "run-signature",
		TicketID:      ticket.ID,
		Project:       ticket.Project,
		Status:        contracts.RunStatusActive,
		Kind:          contracts.RunKindWork,
		CreatedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Runs.SaveRun(ctx, run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	evidence := contracts.EvidenceItem{
		EvidenceID:    "evidence-signature",
		RunID:         run.RunID,
		TicketID:      ticket.ID,
		Type:          contracts.EvidenceTypeNote,
		Title:         "release proof",
		Body:          "tests passed",
		Actor:         contracts.Actor("human:owner"),
		CreatedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Evidence.SaveEvidence(ctx, evidence); err != nil {
		t.Fatalf("save evidence: %v", err)
	}
	handoff := contracts.HandoffPacket{
		HandoffID:     "handoff-signature",
		SourceRunID:   run.RunID,
		TicketID:      ticket.ID,
		Actor:         contracts.Actor("human:owner"),
		StatusSummary: "ready for owner",
		GeneratedAt:   actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Handoffs.SaveHandoff(ctx, handoff); err != nil {
		t.Fatalf("save handoff: %v", err)
	}
	gate := contracts.GateSnapshot{
		GateID:        "gate-signature",
		TicketID:      ticket.ID,
		RunID:         run.RunID,
		Kind:          contracts.GateKindReview,
		State:         contracts.GateStateApproved,
		CreatedBy:     contracts.Actor("human:owner"),
		DecidedBy:     contracts.Actor("human:owner"),
		CreatedAt:     actions.now(),
		DecidedAt:     actions.now(),
		SchemaVersion: contracts.CurrentSchemaVersion,
	}
	if err := actions.Gates.SaveGate(ctx, gate); err != nil {
		t.Fatalf("save gate: %v", err)
	}

	for _, item := range []struct {
		name   string
		sign   func(context.Context, string, string, contracts.Actor, string) (SignatureDetailView, error)
		verify func(context.Context, string) (ArtifactSignatureVerifyView, error)
		id     string
		kind   contracts.ArtifactKind
	}{
		{"evidence", actions.SignEvidencePacket, actions.VerifyEvidencePacketSignature, evidence.EvidenceID, contracts.ArtifactKindEvidencePacket},
		{"handoff", actions.SignHandoff, actions.VerifyHandoffSignature, handoff.HandoffID, contracts.ArtifactKindHandoff},
		{"approval", actions.SignApproval, actions.VerifyApprovalSignature, gate.GateID, contracts.ArtifactKindApproval},
	} {
		t.Run(item.name, func(t *testing.T) {
			signed, err := item.sign(ctx, item.id, key.PublicKey.PublicKeyID, contracts.Actor("human:owner"), "sign "+item.name)
			if err != nil {
				t.Fatalf("sign %s: %v", item.name, err)
			}
			if signed.ArtifactKind != item.kind {
				t.Fatalf("unexpected signature kind: %#v", signed)
			}
			verified, err := item.verify(ctx, item.id)
			if err != nil {
				t.Fatalf("verify %s: %v", item.name, err)
			}
			if verified.Signature.State != contracts.VerificationTrustedValid {
				t.Fatalf("%s should verify trusted, got %#v", item.name, verified.Signature)
			}
		})
	}
}
