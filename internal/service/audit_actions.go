package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type AuditReportDetailView struct {
	Kind        string                `json:"kind"`
	GeneratedAt time.Time             `json:"generated_at"`
	Report      contracts.AuditReport `json:"report"`
}

type AuditReportListView struct {
	Kind        string                  `json:"kind"`
	GeneratedAt time.Time               `json:"generated_at"`
	Items       []contracts.AuditReport `json:"items"`
}

type AuditReportExportResultView struct {
	Kind        string                `json:"kind"`
	GeneratedAt time.Time             `json:"generated_at"`
	Packet      contracts.AuditPacket `json:"packet"`
	Path        string                `json:"path"`
}

type AuditIntegrityView struct {
	Kind               string   `json:"kind"`
	Verified           bool     `json:"verified"`
	ArtifactType       string   `json:"artifact_type"`
	ArtifactUID        string   `json:"artifact_uid"`
	PacketHash         string   `json:"packet_hash,omitempty"`
	ExpectedPacketHash string   `json:"expected_packet_hash,omitempty"`
	Errors             []string `json:"errors,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
}

type AuditPolicyExplanationView struct {
	Kind             string                           `json:"kind"`
	GeneratedAt      time.Time                        `json:"generated_at"`
	Target           string                           `json:"target"`
	Event            *contracts.Event                 `json:"event,omitempty"`
	Policies         []contracts.GovernancePolicy     `json:"policies,omitempty"`
	Governance       *contracts.GovernanceExplanation `json:"governance,omitempty"`
	Inputs           map[string]string                `json:"inputs,omitempty"`
	ReasonCodes      []string                         `json:"reason_codes,omitempty"`
	SnapshotGuidance string                           `json:"snapshot_guidance"`
}

type auditScope struct {
	Kind    contracts.AuditScopeKind
	ID      string
	Project string
}

func (s *ActionService) CreateAuditReport(ctx context.Context, scopeRaw string, actor contracts.Actor, reason string) (AuditReportDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "create audit report", func(ctx context.Context) (AuditReportDetailView, error) {
		if !actor.IsValid() {
			return AuditReportDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return AuditReportDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		scope, err := s.resolveAuditScope(ctx, scopeRaw)
		if err != nil {
			return AuditReportDetailView{}, err
		}
		events, err := s.auditEventsForScope(ctx, scope)
		if err != nil {
			return AuditReportDetailView{}, err
		}
		policyHash, err := s.auditPolicySnapshotHash(ctx)
		if err != nil {
			return AuditReportDetailView{}, err
		}
		trustHash, err := s.auditTrustSnapshotHash(ctx)
		if err != nil {
			return AuditReportDetailView{}, err
		}
		artifacts, err := s.auditArtifactHashes(ctx, scope)
		if err != nil {
			return AuditReportDetailView{}, err
		}
		now := s.now()
		report := contracts.AuditReport{
			AuditReportID:          "audit-" + NewOpaqueID(),
			ScopeKind:              scope.Kind,
			ScopeID:                scope.ID,
			GeneratedAt:            now,
			GeneratedBy:            actor,
			EventRange:             auditEventRange(events),
			PolicySnapshotHash:     policyHash,
			TrustSnapshotHash:      trustHash,
			IncludedArtifactHashes: artifacts,
			Findings:               auditFindings(events, artifacts),
			SchemaVersion:          contracts.CurrentSchemaVersion,
		}
		report = normalizeAuditReport(report)
		eventProject := workspaceProjectKey
		if scope.Project != "" {
			eventProject = scope.Project
		}
		event, err := s.newEvent(ctx, eventProject, now, actor, reason, contracts.EventAuditReportCreated, auditScopeTicketID(scope), report)
		if err != nil {
			return AuditReportDetailView{}, err
		}
		if err := s.commitMutation(ctx, "create audit report", "audit_report", event, func(ctx context.Context) error {
			return s.AuditReports.SaveAuditReport(ctx, report)
		}); err != nil {
			return AuditReportDetailView{}, err
		}
		return AuditReportDetailView{Kind: "audit_report_detail", GeneratedAt: s.now(), Report: report}, nil
	})
}

func (s *ActionService) ListAuditReports(ctx context.Context) (AuditReportListView, error) {
	items, err := s.AuditReports.ListAuditReports(ctx)
	if err != nil {
		return AuditReportListView{}, err
	}
	return AuditReportListView{Kind: "audit_report_list", GeneratedAt: s.now(), Items: items}, nil
}

func (s *ActionService) AuditReportDetail(ctx context.Context, reportID string) (AuditReportDetailView, error) {
	report, err := s.AuditReports.LoadAuditReport(ctx, reportID)
	if err != nil {
		return AuditReportDetailView{}, err
	}
	return AuditReportDetailView{Kind: "audit_report_detail", GeneratedAt: s.now(), Report: report}, nil
}

func (s *ActionService) auditEventProjectForReport(ctx context.Context, report contracts.AuditReport) (string, error) {
	switch report.ScopeKind {
	case contracts.AuditScopeWorkspace, contracts.AuditScopeRelease, contracts.AuditScopeIncident:
		return workspaceProjectKey, nil
	case contracts.AuditScopeProject:
		return strings.TrimSpace(report.ScopeID), nil
	case contracts.AuditScopeTicket:
		ticket, err := s.Tickets.GetTicket(ctx, report.ScopeID)
		if err != nil {
			return "", err
		}
		return ticket.Project, nil
	case contracts.AuditScopeRun:
		run, err := s.Runs.LoadRun(ctx, report.ScopeID)
		if err != nil {
			return "", err
		}
		return run.Project, nil
	case contracts.AuditScopeChange:
		change, err := s.Changes.LoadChange(ctx, report.ScopeID)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(change.TicketID) != "" {
			ticket, err := s.Tickets.GetTicket(ctx, change.TicketID)
			if err != nil {
				return "", err
			}
			return ticket.Project, nil
		}
		if strings.TrimSpace(change.RunID) != "" {
			run, err := s.Runs.LoadRun(ctx, change.RunID)
			if err != nil {
				return "", err
			}
			return run.Project, nil
		}
		return workspaceProjectKey, nil
	default:
		return "", apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported audit scope kind: %s", report.ScopeKind))
	}
}

func (s *ActionService) ExportAuditPacket(ctx context.Context, reportID string, actor contracts.Actor, reason string) (AuditReportExportResultView, error) {
	return withWriteLock(ctx, s.LockManager, "export audit packet", func(ctx context.Context) (AuditReportExportResultView, error) {
		if !actor.IsValid() {
			return AuditReportExportResultView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return AuditReportExportResultView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		report, err := s.AuditReports.LoadAuditReport(ctx, reportID)
		if err != nil {
			return AuditReportExportResultView{}, err
		}
		hash, err := auditReportPayloadHash(report)
		if err != nil {
			return AuditReportExportResultView{}, err
		}
		packet := normalizeAuditPacket(contracts.AuditPacket{
			PacketID:         "packet-" + NewOpaqueID(),
			Report:           report,
			Canonicalization: contracts.CanonicalizationAtlasV1,
			PacketHash:       hash,
			SchemaVersion:    contracts.CurrentSchemaVersion,
		})
		scope := auditScope{Kind: report.ScopeKind, ID: report.ScopeID}
		eventProject, err := s.auditEventProjectForReport(ctx, report)
		if err != nil {
			return AuditReportExportResultView{}, err
		}
		scope.Project = eventProject
		event, err := s.newEvent(ctx, eventProject, s.now(), actor, reason, contracts.EventAuditReportExported, auditScopeTicketID(scope), auditExportEventPayload(ctx, s, report, packet))
		if err != nil {
			return AuditReportExportResultView{}, err
		}
		if err := s.commitMutation(ctx, "export audit packet", "audit_packet", event, func(ctx context.Context) error {
			return s.AuditPackets.SaveAuditPacket(ctx, packet)
		}); err != nil {
			return AuditReportExportResultView{}, err
		}
		return AuditReportExportResultView{Kind: "audit_report_export_result", GeneratedAt: s.now(), Packet: packet, Path: auditPacketPath(s.Root, packet.PacketID)}, nil
	})
}

func auditExportEventPayload(ctx context.Context, s *ActionService, report contracts.AuditReport, packet contracts.AuditPacket) map[string]string {
	payload := map[string]string{
		"audit_report_id": report.AuditReportID,
		"packet_id":       packet.PacketID,
		"packet_hash":     packet.PacketHash,
		"scope_kind":      string(report.ScopeKind),
	}
	if strings.TrimSpace(report.ScopeID) != "" {
		payload["scope_id"] = report.ScopeID
	}
	switch report.ScopeKind {
	case contracts.AuditScopeTicket:
		payload["ticket_id"] = report.ScopeID
	case contracts.AuditScopeRun:
		payload["run_id"] = report.ScopeID
		if run, err := s.Runs.LoadRun(ctx, report.ScopeID); err == nil && strings.TrimSpace(run.TicketID) != "" {
			payload["ticket_id"] = run.TicketID
		}
	case contracts.AuditScopeChange:
		payload["change_id"] = report.ScopeID
		if change, err := s.Changes.LoadChange(ctx, report.ScopeID); err == nil {
			if strings.TrimSpace(change.TicketID) != "" {
				payload["ticket_id"] = change.TicketID
			}
			if strings.TrimSpace(change.RunID) != "" {
				payload["run_id"] = change.RunID
			}
		}
	case contracts.AuditScopeProject:
		payload["project_id"] = report.ScopeID
	case contracts.AuditScopeRelease:
		payload["release_id"] = report.ScopeID
	case contracts.AuditScopeIncident:
		payload["incident_id"] = report.ScopeID
	}
	return payload
}

func (s *ActionService) VerifyAuditArtifact(ctx context.Context, ref string) (ArtifactSignatureVerifyView, error) {
	return s.verifyAuditArtifactAs(ctx, ref, "")
}

func (s *ActionService) VerifyAuditReportArtifact(ctx context.Context, ref string) (ArtifactSignatureVerifyView, error) {
	return s.verifyAuditArtifactAs(ctx, ref, contracts.ArtifactKindAuditReport)
}

func (s *ActionService) VerifyAuditPacketArtifact(ctx context.Context, ref string) (ArtifactSignatureVerifyView, error) {
	return s.verifyAuditArtifactAs(ctx, ref, contracts.ArtifactKindAuditPacket)
}

func (s *ActionService) verifyAuditArtifactAs(ctx context.Context, ref string, expected contracts.ArtifactKind) (ArtifactSignatureVerifyView, error) {
	report, packet, artifactType, err := s.resolveAuditArtifact(ctx, ref)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	if expected == contracts.ArtifactKindAuditReport && packet != nil {
		return ArtifactSignatureVerifyView{}, apperr.New(apperr.CodeInvalidInput, "expected audit report, got audit packet")
	}
	if expected == contracts.ArtifactKindAuditPacket && report != nil {
		return ArtifactSignatureVerifyView{}, apperr.New(apperr.CodeInvalidInput, "expected audit packet, got audit report")
	}
	integrity := AuditIntegrityView{Kind: "audit_integrity", Verified: true, ArtifactType: artifactType}
	var payload any
	var envelopes []contracts.SignatureEnvelope
	var kind contracts.ArtifactKind
	var uid string
	viewKind := "audit_report_verify_result"
	if packet != nil {
		expected, err := auditReportPayloadHash(packet.Report)
		if err != nil {
			return ArtifactSignatureVerifyView{}, err
		}
		integrity.ArtifactUID = packet.PacketID
		integrity.PacketHash = packet.PacketHash
		integrity.ExpectedPacketHash = expected
		if packet.PacketHash != expected {
			integrity.Verified = false
			integrity.Errors = append(integrity.Errors, "packet_hash_mismatch")
		}
		payload = auditPacketSignaturePayload(*packet)
		envelopes = packet.SignatureEnvelopes
		kind = contracts.ArtifactKindAuditPacket
		uid = packet.PacketID
		viewKind = "audit_packet_verify_result"
	} else {
		integrity.ArtifactUID = report.AuditReportID
		if err := report.Validate(); err != nil {
			integrity.Verified = false
			integrity.Errors = append(integrity.Errors, "audit_report_invalid")
		}
		payload = auditReportSignaturePayload(*report)
		envelopes = report.SignatureEnvelopes
		kind = contracts.ArtifactKindAuditReport
		uid = report.AuditReportID
	}
	result, err := s.VerifyPayloadSignatures(ctx, payload, envelopes, kind, uid)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	return ArtifactSignatureVerifyView{Kind: viewKind, Integrity: integrity, Signature: result, GeneratedAt: s.now()}, nil
}

func (s *ActionService) SignAuditReport(ctx context.Context, reportID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return s.signStoredAuditArtifact(ctx, reportID, contracts.ArtifactKindAuditReport, publicKeyID, actor, reason)
}

func (s *ActionService) SignAuditPacket(ctx context.Context, packetID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return s.signStoredAuditArtifact(ctx, packetID, contracts.ArtifactKindAuditPacket, publicKeyID, actor, reason)
}

func (s *ActionService) SignApproval(ctx context.Context, gateID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return s.signStoredPayload(ctx, contracts.ArtifactKindApproval, gateID, publicKeyID, actor, reason, func(ctx context.Context) (any, error) {
		return s.Gates.LoadGate(ctx, gateID)
	})
}

func (s *ActionService) VerifyApprovalSignature(ctx context.Context, gateID string) (ArtifactSignatureVerifyView, error) {
	return s.verifyStoredPayloadSignature(ctx, contracts.ArtifactKindApproval, gateID, "signature_verify_result", func(ctx context.Context) (any, error) {
		return s.Gates.LoadGate(ctx, gateID)
	})
}

func (s *ActionService) SignHandoff(ctx context.Context, handoffID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return s.signStoredPayload(ctx, contracts.ArtifactKindHandoff, handoffID, publicKeyID, actor, reason, func(ctx context.Context) (any, error) {
		return s.Handoffs.LoadHandoff(ctx, handoffID)
	})
}

func (s *ActionService) VerifyHandoffSignature(ctx context.Context, handoffID string) (ArtifactSignatureVerifyView, error) {
	return s.verifyStoredPayloadSignature(ctx, contracts.ArtifactKindHandoff, handoffID, "signature_verify_result", func(ctx context.Context) (any, error) {
		return s.Handoffs.LoadHandoff(ctx, handoffID)
	})
}

func (s *ActionService) SignEvidencePacket(ctx context.Context, evidenceID string, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return s.signStoredPayload(ctx, contracts.ArtifactKindEvidencePacket, evidenceID, publicKeyID, actor, reason, func(ctx context.Context) (any, error) {
		return s.Evidence.LoadEvidence(ctx, evidenceID)
	})
}

func (s *ActionService) VerifyEvidencePacketSignature(ctx context.Context, evidenceID string) (ArtifactSignatureVerifyView, error) {
	return s.verifyStoredPayloadSignature(ctx, contracts.ArtifactKindEvidencePacket, evidenceID, "evidence_verify_result", func(ctx context.Context) (any, error) {
		return s.Evidence.LoadEvidence(ctx, evidenceID)
	})
}

func (s *ActionService) ExplainAuditPolicy(ctx context.Context, target string) (AuditPolicyExplanationView, error) {
	events, err := s.Events.StreamEvents(ctx, "", 0)
	if err != nil {
		return AuditPolicyExplanationView{}, err
	}
	target = strings.TrimSpace(target)
	lookup := strings.TrimPrefix(target, "event:")
	if lookup == "" {
		return AuditPolicyExplanationView{}, apperr.New(apperr.CodeInvalidInput, "event_uid is required")
	}
	if auditLooksNumeric(lookup) {
		return AuditPolicyExplanationView{}, apperr.New(apperr.CodeInvalidInput, "event_uid is required; numeric event IDs are project-scoped")
	}
	var found *contracts.Event
	for idx := range events {
		if events[idx].EventUID == lookup {
			event := events[idx]
			found = &event
			break
		}
	}
	if found == nil {
		return AuditPolicyExplanationView{}, apperr.New(apperr.CodeNotFound, fmt.Sprintf("policy provenance target %s not found", lookup))
	}
	policies, err := s.GovernancePolicies.ListGovernancePolicies(ctx)
	if err != nil {
		return AuditPolicyExplanationView{}, err
	}
	reasonCodes := []string{"event_loaded", "policy_snapshot_is_current_not_historical"}
	if len(policies) == 0 {
		reasonCodes = append(reasonCodes, "no_governance_policies")
	}
	input, inferred := auditGovernanceInputForEvent(*found)
	var governance *contracts.GovernanceExplanation
	inputs := map[string]string{}
	if inferred {
		reasonCodes = append(reasonCodes, "protected_action_inferred")
		explained, err := s.ExplainGovernance(ctx, input)
		if err != nil {
			return AuditPolicyExplanationView{}, err
		}
		governance = &explained
		inputs = map[string]string{
			"action": string(explained.Action),
			"actor":  string(explained.Actor),
			"target": explained.Target,
		}
		for key, value := range explained.Inputs {
			inputs[key] = value
		}
		if explained.Allowed {
			reasonCodes = append(reasonCodes, "governance_allowed")
		} else {
			reasonCodes = append(reasonCodes, "governance_denied")
		}
		if stringSliceContains(explained.ReasonCodes, "owner_override_applied") {
			reasonCodes = append(reasonCodes, "governance_override_applied")
		}
	} else {
		reasonCodes = append(reasonCodes, "protected_action_not_inferred")
	}
	return AuditPolicyExplanationView{
		Kind:             "governance_explanation",
		GeneratedAt:      s.now(),
		Target:           lookup,
		Event:            found,
		Policies:         policies,
		Governance:       governance,
		Inputs:           inputs,
		ReasonCodes:      reasonCodes,
		SnapshotGuidance: "audit reports bind policy_snapshot_hash; this explanation uses the current local policy store for operator context",
	}, nil
}

func (s *ActionService) signStoredAuditArtifact(ctx context.Context, ref string, kind contracts.ArtifactKind, publicKeyID string, actor contracts.Actor, reason string) (SignatureDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sign audit artifact", func(ctx context.Context) (SignatureDetailView, error) {
		if !actor.IsValid() {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		var payload any
		var artifactUID string
		var save func(context.Context, contracts.SignatureEnvelope) error
		switch kind {
		case contracts.ArtifactKindAuditReport:
			report, err := s.AuditReports.LoadAuditReport(ctx, ref)
			if err != nil {
				return SignatureDetailView{}, err
			}
			artifactUID = report.AuditReportID
			payload = auditReportSignaturePayload(report)
			save = func(ctx context.Context, envelope contracts.SignatureEnvelope) error {
				report.SignatureEnvelopes = upsertSignatureEnvelope(report.SignatureEnvelopes, envelope)
				return s.AuditReports.SaveAuditReport(ctx, report)
			}
		case contracts.ArtifactKindAuditPacket:
			packet, err := s.AuditPackets.LoadAuditPacket(ctx, ref)
			if err != nil {
				return SignatureDetailView{}, err
			}
			expectedHash, err := auditReportPayloadHash(packet.Report)
			if err != nil {
				return SignatureDetailView{}, err
			}
			if packet.PacketHash != expectedHash {
				return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "audit packet integrity must verify before signing")
			}
			artifactUID = packet.PacketID
			payload = auditPacketSignaturePayload(packet)
			save = func(ctx context.Context, envelope contracts.SignatureEnvelope) error {
				packet.SignatureEnvelopes = upsertSignatureEnvelope(packet.SignatureEnvelopes, envelope)
				return s.AuditPackets.SaveAuditPacket(ctx, packet)
			}
		default:
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported audit artifact kind: %s", kind))
		}
		envelope, err := s.SignPayload(ctx, SignatureRequest{ArtifactKind: kind, ArtifactUID: artifactUID, PublicKeyID: publicKeyID, Payload: payload})
		if err != nil {
			return SignatureDetailView{}, err
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventSignatureCreated, "", envelope)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if err := s.commitMutation(ctx, "sign audit artifact", "signature", event, func(ctx context.Context) error {
			if err := save(ctx, envelope); err != nil {
				return err
			}
			return s.Signatures.SaveSignature(ctx, envelope)
		}); err != nil {
			return SignatureDetailView{}, err
		}
		return SignatureDetailView{Kind: "signature_detail", ArtifactKind: envelope.ArtifactKind, ArtifactUID: envelope.ArtifactUID, Signature: envelope, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) signStoredPayload(ctx context.Context, kind contracts.ArtifactKind, artifactUID string, publicKeyID string, actor contracts.Actor, reason string, load func(context.Context) (any, error)) (SignatureDetailView, error) {
	return withWriteLock(ctx, s.LockManager, "sign artifact", func(ctx context.Context) (SignatureDetailView, error) {
		if !actor.IsValid() {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if strings.TrimSpace(reason) == "" {
			return SignatureDetailView{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
		}
		payload, err := load(ctx)
		if err != nil {
			return SignatureDetailView{}, err
		}
		artifactUID = strings.TrimSpace(artifactUID)
		envelope, err := s.SignPayload(ctx, SignatureRequest{ArtifactKind: kind, ArtifactUID: artifactUID, PublicKeyID: publicKeyID, Payload: payload})
		if err != nil {
			return SignatureDetailView{}, err
		}
		event, err := s.newEvent(ctx, workspaceProjectKey, s.now(), actor, reason, contracts.EventSignatureCreated, "", envelope)
		if err != nil {
			return SignatureDetailView{}, err
		}
		if err := s.commitMutation(ctx, "sign artifact", "signature", event, func(ctx context.Context) error {
			return s.Signatures.SaveSignature(ctx, envelope)
		}); err != nil {
			return SignatureDetailView{}, err
		}
		return SignatureDetailView{Kind: "signature_detail", ArtifactKind: envelope.ArtifactKind, ArtifactUID: envelope.ArtifactUID, Signature: envelope, GeneratedAt: s.now()}, nil
	})
}

func (s *ActionService) verifyStoredPayloadSignature(ctx context.Context, kind contracts.ArtifactKind, artifactUID string, viewKind string, load func(context.Context) (any, error)) (ArtifactSignatureVerifyView, error) {
	payload, err := load(ctx)
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	envelopes, err := s.signaturesForArtifact(ctx, kind, strings.TrimSpace(artifactUID))
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	result, err := s.VerifyPayloadSignatures(ctx, payload, envelopes, kind, strings.TrimSpace(artifactUID))
	if err != nil {
		return ArtifactSignatureVerifyView{}, err
	}
	return ArtifactSignatureVerifyView{Kind: viewKind, Signature: result, GeneratedAt: s.now()}, nil
}

func (s *ActionService) signaturesForArtifact(ctx context.Context, kind contracts.ArtifactKind, uid string) ([]contracts.SignatureEnvelope, error) {
	all, err := s.Signatures.ListSignatures(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]contracts.SignatureEnvelope, 0)
	for _, envelope := range all {
		if envelope.ArtifactKind == kind && envelope.ArtifactUID == uid {
			items = append(items, envelope)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SignedAt.Equal(items[j].SignedAt) {
			return items[i].SignatureID < items[j].SignatureID
		}
		return items[i].SignedAt.Before(items[j].SignedAt)
	})
	return items, nil
}

func (s *ActionService) resolveAuditArtifact(ctx context.Context, ref string) (*contracts.AuditReport, *contracts.AuditPacket, string, error) {
	if raw, ok, err := readAuditRefPath(s.Root, ref); err != nil {
		return nil, nil, "", err
	} else if ok {
		var packet contracts.AuditPacket
		if json.Unmarshal(raw, &packet) == nil && strings.TrimSpace(packet.PacketID) != "" {
			packet, err := decodeAuditPacket(raw, ref)
			if err != nil {
				return nil, nil, "", err
			}
			return nil, &packet, "packet", nil
		}
		report, err := decodeAuditReport(raw, ref)
		if err != nil {
			return nil, nil, "", err
		}
		return &report, nil, "report", nil
	}
	if packet, err := s.AuditPackets.LoadAuditPacket(ctx, ref); err == nil {
		return nil, &packet, "packet", nil
	}
	report, err := s.AuditReports.LoadAuditReport(ctx, ref)
	if err != nil {
		return nil, nil, "", err
	}
	return &report, nil, "report", nil
}

func readAuditRefPath(root string, ref string) ([]byte, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, false, nil
	}
	if !strings.Contains(ref, string(os.PathSeparator)) && !strings.HasSuffix(ref, ".json") {
		return nil, false, nil
	}
	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, false, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, false, err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return nil, false, apperr.New(apperr.CodeInvalidInput, "audit artifact path must stay inside the workspace")
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read audit artifact %s: %w", ref, err)
	}
	return raw, true, nil
}

func (s *ActionService) resolveAuditScope(ctx context.Context, raw string) (auditScope, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "workspace" {
		return auditScope{Kind: contracts.AuditScopeWorkspace}, nil
	}
	kind, id, ok := strings.Cut(raw, ":")
	if !ok {
		return auditScope{}, apperr.New(apperr.CodeInvalidInput, "audit scope must be workspace or kind:id")
	}
	id = strings.TrimSpace(id)
	switch strings.TrimSpace(kind) {
	case "project":
		if id == "" {
			return auditScope{}, apperr.New(apperr.CodeInvalidInput, "project audit scope requires an id")
		}
		return auditScope{Kind: contracts.AuditScopeProject, ID: id, Project: id}, nil
	case "ticket":
		ticket, err := s.Tickets.GetTicket(ctx, id)
		if err != nil {
			return auditScope{}, err
		}
		return auditScope{Kind: contracts.AuditScopeTicket, ID: id, Project: ticket.Project}, nil
	case "run":
		run, err := s.Runs.LoadRun(ctx, id)
		if err != nil {
			return auditScope{}, err
		}
		return auditScope{Kind: contracts.AuditScopeRun, ID: id, Project: run.Project}, nil
	case "change":
		change, err := s.Changes.LoadChange(ctx, id)
		if err != nil {
			return auditScope{}, err
		}
		project := ""
		if strings.TrimSpace(change.RunID) != "" {
			if run, err := s.Runs.LoadRun(ctx, change.RunID); err == nil {
				project = run.Project
			}
		}
		if project == "" {
			if ticket, err := s.Tickets.GetTicket(ctx, change.TicketID); err == nil {
				project = ticket.Project
			}
		}
		return auditScope{Kind: contracts.AuditScopeChange, ID: id, Project: project}, nil
	case "release":
		return auditScope{Kind: contracts.AuditScopeRelease, ID: id}, nil
	case "incident":
		return auditScope{Kind: contracts.AuditScopeIncident, ID: id}, nil
	default:
		return auditScope{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported audit scope kind: %s", kind))
	}
}

func (s *ActionService) auditEventsForScope(ctx context.Context, scope auditScope) ([]contracts.Event, error) {
	streamProject := ""
	if scope.Kind == contracts.AuditScopeProject || scope.Kind == contracts.AuditScopeTicket || scope.Kind == contracts.AuditScopeRun || scope.Kind == contracts.AuditScopeChange {
		streamProject = scope.Project
	}
	events, err := s.Events.StreamEvents(ctx, streamProject, 0)
	if err != nil {
		return nil, err
	}
	filtered := make([]contracts.Event, 0, len(events))
	for _, event := range events {
		if auditEventMatchesScope(event, scope) {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func auditEventMatchesScope(event contracts.Event, scope auditScope) bool {
	switch scope.Kind {
	case contracts.AuditScopeWorkspace:
		return true
	case contracts.AuditScopeProject:
		return event.Project == scope.Project
	case contracts.AuditScopeTicket:
		return event.TicketID == scope.ID || auditPayloadReferencesID(event.Payload, scope.ID)
	case contracts.AuditScopeRun, contracts.AuditScopeChange, contracts.AuditScopeRelease, contracts.AuditScopeIncident:
		return auditPayloadReferencesID(event.Payload, scope.ID)
	default:
		return false
	}
}

func auditPayloadReferencesID(payload any, needle string) bool {
	if strings.TrimSpace(needle) == "" || payload == nil {
		return false
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return false
	}
	return auditAnyStringEquals(decoded, needle)
}

func auditAnyStringEquals(value any, needle string) bool {
	switch item := value.(type) {
	case string:
		return item == needle
	case []any:
		for _, child := range item {
			if auditAnyStringEquals(child, needle) {
				return true
			}
		}
	case map[string]any:
		for _, child := range item {
			if auditAnyStringEquals(child, needle) {
				return true
			}
		}
	}
	return false
}

func auditLooksNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func auditGovernanceInputForEvent(event contracts.Event) (GovernanceEvaluationInput, bool) {
	input := GovernanceEvaluationInput{Actor: event.Actor, Reason: event.Reason}
	workspace := func(action contracts.ProtectedAction) (GovernanceEvaluationInput, bool) {
		input.Action = action
		input.Target = "workspace"
		return input, true
	}
	switch event.Type {
	case contracts.EventTicketMoved:
		if auditPayloadString(event.Payload, "to") != string(contracts.StatusDone) {
			return GovernanceEvaluationInput{}, false
		}
		ticketID := firstNonEmpty(event.TicketID, auditPayloadString(event.Payload, "ticket_id"))
		if ticketID == "" {
			return GovernanceEvaluationInput{}, false
		}
		input.Action = contracts.ProtectedActionTicketComplete
		input.Target = "ticket:" + ticketID
		input.TicketID = ticketID
		return input, true
	case contracts.EventRunCompleted:
		runID := auditPayloadString(event.Payload, "run_id")
		if runID == "" {
			return GovernanceEvaluationInput{}, false
		}
		input.Action = contracts.ProtectedActionRunComplete
		input.Target = "run:" + runID
		input.RunID = runID
		input.TicketID = event.TicketID
		return input, true
	case contracts.EventGateApproved:
		gateID := auditPayloadString(event.Payload, "gate_id")
		if gateID == "" {
			return GovernanceEvaluationInput{}, false
		}
		input.Action = contracts.ProtectedActionGateApprove
		input.Target = "gate:" + gateID
		input.GateID = gateID
		input.TicketID = event.TicketID
		return input, true
	case contracts.EventGateWaived:
		gateID := auditPayloadString(event.Payload, "gate_id")
		if gateID == "" {
			return GovernanceEvaluationInput{}, false
		}
		input.Action = contracts.ProtectedActionGateWaive
		input.Target = "gate:" + gateID
		input.GateID = gateID
		input.TicketID = event.TicketID
		return input, true
	case contracts.EventChangeMerged:
		changeID := auditPayloadString(event.Payload, "change_id")
		if changeID == "" {
			return GovernanceEvaluationInput{}, false
		}
		input.Action = contracts.ProtectedActionChangeMerge
		input.Target = "change:" + changeID
		input.ChangeID = changeID
		input.TicketID = event.TicketID
		return input, true
	case contracts.EventExportCreated, contracts.EventRedactionExported:
		return workspace(contracts.ProtectedActionExportCreate)
	case contracts.EventArchiveApplied:
		return workspace(contracts.ProtectedActionArchiveApply)
	case contracts.EventArchiveRestored:
		return workspace(contracts.ProtectedActionArchiveRestore)
	case contracts.EventBackupRestored:
		return workspace(contracts.ProtectedActionBackupRestore)
	case contracts.EventTrustBound:
		return workspace(contracts.ProtectedActionTrustKey)
	case contracts.EventKeyRevoked, contracts.EventTrustRevoked:
		return workspace(contracts.ProtectedActionRevokeKey)
	case contracts.EventImportApplied:
		return workspace(contracts.ProtectedActionImportApply)
	case contracts.EventBundleImported, contracts.EventSyncCompleted:
		return workspace(contracts.ProtectedActionSyncImportApply)
	case contracts.EventRedactionPreviewed:
		return workspace(contracts.ProtectedActionRedactionOverride)
	default:
		return GovernanceEvaluationInput{}, false
	}
}

func auditPayloadString(payload any, keys ...string) string {
	if len(keys) == 0 || payload == nil {
		return ""
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return ""
	}
	allowed := map[string]struct{}{}
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	return auditPayloadStringFromAny(decoded, allowed)
}

func auditPayloadStringFromAny(value any, keys map[string]struct{}) string {
	switch item := value.(type) {
	case string:
		return ""
	case []any:
		for _, child := range item {
			if found := auditPayloadStringFromAny(child, keys); found != "" {
				return found
			}
		}
	case map[string]any:
		for key, child := range item {
			if _, ok := keys[key]; ok {
				if value, ok := child.(string); ok {
					return strings.TrimSpace(value)
				}
			}
		}
		for _, child := range item {
			if found := auditPayloadStringFromAny(child, keys); found != "" {
				return found
			}
		}
	}
	return ""
}

func auditEventRange(events []contracts.Event) contracts.EventRange {
	var out contracts.EventRange
	for idx, event := range events {
		if idx == 0 || event.EventID < out.FromEventID {
			out.FromEventID = event.EventID
			out.FromTime = event.Timestamp
		}
		if event.EventID > out.ToEventID {
			out.ToEventID = event.EventID
			out.ToTime = event.Timestamp
		}
	}
	return out
}

func (s *ActionService) auditScopeArtifactIDs(ctx context.Context, scope auditScope) (map[string]struct{}, error) {
	ids := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				ids[value] = struct{}{}
			}
		}
	}
	add(scope.ID)
	switch scope.Kind {
	case contracts.AuditScopeWorkspace, contracts.AuditScopeRelease, contracts.AuditScopeIncident:
		return ids, nil
	case contracts.AuditScopeProject:
		if err := s.addProjectAuditArtifactIDs(ctx, scope.ID, ids); err != nil {
			return nil, err
		}
	case contracts.AuditScopeTicket:
		if err := s.addTicketAuditArtifactIDs(ctx, scope.ID, ids); err != nil {
			return nil, err
		}
	case contracts.AuditScopeRun:
		run, err := s.Runs.LoadRun(ctx, scope.ID)
		if err != nil {
			return nil, err
		}
		add(run.RunID, run.TicketID)
		if err := s.addRunAuditArtifactIDs(ctx, run, ids); err != nil {
			return nil, err
		}
	case contracts.AuditScopeChange:
		change, err := s.Changes.LoadChange(ctx, scope.ID)
		if err != nil {
			return nil, err
		}
		add(change.ChangeID, change.TicketID, change.RunID)
		if strings.TrimSpace(change.TicketID) != "" {
			if err := s.addTicketAuditArtifactIDs(ctx, change.TicketID, ids); err != nil {
				return nil, err
			}
		}
	default:
		return nil, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported audit scope kind: %s", scope.Kind))
	}
	return ids, nil
}

func (s *ActionService) addProjectAuditArtifactIDs(ctx context.Context, project string, ids map[string]struct{}) error {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil
	}
	ids[project] = struct{}{}
	tickets, err := s.Tickets.ListTickets(ctx, contracts.TicketListOptions{Project: project, IncludeArchived: true})
	if err != nil {
		return err
	}
	for _, ticket := range tickets {
		if err := s.addTicketAuditArtifactIDs(ctx, ticket.ID, ids); err != nil {
			return err
		}
	}
	return nil
}

func (s *ActionService) addTicketAuditArtifactIDs(ctx context.Context, ticketID string, ids map[string]struct{}) error {
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				ids[value] = struct{}{}
			}
		}
	}
	ticket, err := s.Tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	add(ticket.ID)
	runs, err := s.Runs.ListRuns(ctx, ticket.ID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		add(run.RunID, run.TicketID)
		if err := s.addRunAuditArtifactIDs(ctx, run, ids); err != nil {
			return err
		}
	}
	gates, err := s.Gates.ListGates(ctx, ticket.ID)
	if err != nil {
		return err
	}
	for _, gate := range gates {
		add(gate.GateID, gate.TicketID, gate.RunID)
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, ticket.ID)
	if err != nil {
		return err
	}
	for _, handoff := range handoffs {
		add(handoff.HandoffID, handoff.TicketID, handoff.SourceRunID)
	}
	changes, err := s.Changes.ListChanges(ctx, ticket.ID)
	if err != nil {
		return err
	}
	for _, change := range changes {
		add(change.ChangeID, change.TicketID, change.RunID)
	}
	return nil
}

func (s *ActionService) addRunAuditArtifactIDs(ctx context.Context, run contracts.RunSnapshot, ids map[string]struct{}) error {
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				ids[value] = struct{}{}
			}
		}
	}
	evidence, err := s.Evidence.ListEvidence(ctx, run.RunID)
	if err != nil {
		return err
	}
	for _, item := range evidence {
		add(item.EvidenceID, item.RunID, item.TicketID)
	}
	if strings.TrimSpace(run.TicketID) == "" {
		return nil
	}
	gates, err := s.Gates.ListGates(ctx, run.TicketID)
	if err != nil {
		return err
	}
	for _, gate := range gates {
		if gate.RunID == run.RunID {
			add(gate.GateID, gate.TicketID, gate.RunID)
		}
	}
	handoffs, err := s.Handoffs.ListHandoffs(ctx, run.TicketID)
	if err != nil {
		return err
	}
	for _, handoff := range handoffs {
		if handoff.SourceRunID == run.RunID {
			add(handoff.HandoffID, handoff.TicketID, handoff.SourceRunID)
		}
	}
	changes, err := s.Changes.ListChanges(ctx, run.TicketID)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if change.RunID == run.RunID {
			add(change.ChangeID, change.TicketID, change.RunID)
		}
	}
	return nil
}

func auditFindings(events []contracts.Event, artifacts []contracts.ArtifactHash) []contracts.AuditFinding {
	findings := []contracts.AuditFinding{{
		FindingID:   "audit-snapshot-created",
		Severity:    contracts.AuditFindingInfo,
		Code:        "snapshot_created",
		Message:     "audit report is a point-in-time snapshot and verification checks this packet, not current workspace meaning",
		ArtifactUID: "",
	}}
	if len(events) == 0 {
		findings = append(findings, contracts.AuditFinding{
			FindingID: "audit-no-events",
			Severity:  contracts.AuditFindingWarning,
			Code:      "empty_scope",
			Message:   "audit scope had no matching events at generation time",
		})
	}
	if len(artifacts) == 0 {
		findings = append(findings, contracts.AuditFinding{
			FindingID: "audit-no-artifacts",
			Severity:  contracts.AuditFindingWarning,
			Code:      "empty_artifact_set",
			Message:   "audit scope had no hashed Atlas-owned files at generation time",
		})
	}
	return findings
}

func (s *ActionService) auditPolicySnapshotHash(ctx context.Context) (string, error) {
	policies, err := s.GovernancePolicies.ListGovernancePolicies(ctx)
	if err != nil {
		return "", err
	}
	packs, err := s.GovernancePacks.ListGovernancePacks(ctx)
	if err != nil {
		return "", err
	}
	return canonicalSnapshotHash(map[string]any{"policies": policies, "packs": packs})
}

func (s *ActionService) auditTrustSnapshotHash(ctx context.Context) (string, error) {
	keys, err := s.SecurityKeys.ListPublicKeys(ctx)
	if err != nil {
		return "", err
	}
	revocations, err := s.SecurityKeys.ListRevocations(ctx)
	if err != nil {
		return "", err
	}
	bindings, err := s.TrustBindings.ListTrustBindings(ctx)
	if err != nil {
		return "", err
	}
	return canonicalSnapshotHash(map[string]any{"public_keys": keys, "revocations": revocations, "trust_bindings": bindings})
}

func canonicalSnapshotHash(value any) (string, error) {
	raw, err := contracts.CanonicalizeAtlasV1(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s *ActionService) auditArtifactHashes(ctx context.Context, scope auditScope) ([]contracts.ArtifactHash, error) {
	files, err := collectExportFiles(s.Root)
	if err != nil {
		return nil, err
	}
	ids, err := s.auditScopeArtifactIDs(ctx, scope)
	if err != nil {
		return nil, err
	}
	items := make([]contracts.ArtifactHash, 0, len(files))
	for _, rel := range files {
		if !auditFileRelevantToScope(rel, scope, ids) {
			continue
		}
		sum, err := fileSHA256(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		items = append(items, contracts.ArtifactHash{Kind: contracts.ArtifactKindAuditReport, UID: "file:" + rel, SHA256: sum})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UID < items[j].UID })
	return items, nil
}

func auditFileRelevantToScope(rel string, scope auditScope, ids map[string]struct{}) bool {
	rel = filepath.ToSlash(rel)
	switch scope.Kind {
	case contracts.AuditScopeWorkspace:
		return true
	case contracts.AuditScopeProject:
		return auditPathReferencesAnyID(rel, ids) || auditAlwaysRelevantPath(rel)
	case contracts.AuditScopeTicket, contracts.AuditScopeRun, contracts.AuditScopeChange, contracts.AuditScopeRelease, contracts.AuditScopeIncident:
		return auditPathReferencesAnyID(rel, ids) || auditAlwaysRelevantPath(rel)
	default:
		return false
	}
}

func auditAlwaysRelevantPath(rel string) bool {
	return strings.HasPrefix(rel, ".tracker/events/") ||
		strings.HasPrefix(rel, ".tracker/governance/") ||
		strings.HasPrefix(rel, ".tracker/security/") ||
		strings.HasPrefix(rel, ".tracker/classification/")
}

func auditPathReferencesAnyID(rel string, ids map[string]struct{}) bool {
	if len(ids) == 0 {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		candidate := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(segment, ".md"), ".json"), ".toml")
		if _, ok := ids[candidate]; ok {
			return true
		}
	}
	return false
}

func auditScopeTicketID(scope auditScope) string {
	if scope.Kind == contracts.AuditScopeTicket {
		return scope.ID
	}
	return ""
}

func auditReportPayloadHash(report contracts.AuditReport) (string, error) {
	payload := auditReportSignaturePayload(report)
	raw, err := contracts.CanonicalizeAtlasV1(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func auditReportSignaturePayload(report contracts.AuditReport) contracts.AuditReport {
	report = normalizeAuditReport(report)
	report.SignatureEnvelopes = nil
	return report
}

func auditPacketSignaturePayload(packet contracts.AuditPacket) contracts.AuditPacket {
	packet = normalizeAuditPacket(packet)
	packet.SignatureEnvelopes = nil
	return packet
}
