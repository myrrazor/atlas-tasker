package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

const maxEvidenceArtifactBytes = 10 * 1024 * 1024

func (s *ActionService) CheckpointRun(ctx context.Context, runID string, actor contracts.Actor, reason string, title string, body string) (contracts.EvidenceItem, error) {
	return s.AddEvidence(ctx, runID, contracts.EvidenceTypeNote, title, body, "", "", actor, reason, contracts.EventRunCheckpointed)
}

func (s *ActionService) AddEvidence(ctx context.Context, runID string, evidenceType contracts.EvidenceType, title string, body string, artifactSource string, supersedesEvidenceID string, actor contracts.Actor, reason string, eventType contracts.EventType) (contracts.EvidenceItem, error) {
	return withWriteLock(ctx, s.LockManager, "add evidence", func(ctx context.Context) (contracts.EvidenceItem, error) {
		if !actor.IsValid() {
			return contracts.EvidenceItem{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		if !evidenceType.IsValid() {
			return contracts.EvidenceItem{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid evidence type: %s", evidenceType))
		}
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return contracts.EvidenceItem{}, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
		if err != nil {
			return contracts.EvidenceItem{}, err
		}
		if eventType == "" {
			eventType = contracts.EventRunEvidenceAdded
		}
		title = strings.TrimSpace(title)
		body = strings.TrimSpace(body)
		artifactSource = strings.TrimSpace(artifactSource)
		supersedesEvidenceID = strings.TrimSpace(supersedesEvidenceID)
		if title == "" && body == "" && artifactSource == "" {
			return contracts.EvidenceItem{}, apperr.New(apperr.CodeInvalidInput, "evidence requires a title, body, or artifact")
		}
		if supersedesEvidenceID != "" {
			superseded, err := s.Evidence.LoadEvidence(ctx, supersedesEvidenceID)
			if err != nil {
				return contracts.EvidenceItem{}, err
			}
			if superseded.RunID != run.RunID || superseded.TicketID != run.TicketID {
				return contracts.EvidenceItem{}, apperr.New(apperr.CodeConflict, fmt.Sprintf("evidence %s does not belong to run %s", supersedesEvidenceID, run.RunID))
			}
		}
		evidence := contracts.EvidenceItem{
			EvidenceID:           "evidence_" + NewOpaqueID(),
			RunID:                run.RunID,
			TicketID:             run.TicketID,
			Type:                 evidenceType,
			Title:                title,
			Body:                 body,
			SupersedesEvidenceID: supersedesEvidenceID,
			Actor:                actor,
			CreatedAt:            s.now(),
			SchemaVersion:        contracts.CurrentSchemaVersion,
		}
		artifactTarget := ""
		if artifactSource != "" {
			artifactTarget, err = copyEvidenceArtifact(s.Root, run.RunID, evidence.EvidenceID, artifactSource)
			if err != nil {
				return contracts.EvidenceItem{}, err
			}
			evidence.ArtifactPath = artifactTarget
		}
		run.EvidenceCount++
		payload := runMutationPayload{Run: run, Ticket: ticket, Evidence: evidence}
		event, err := s.newEvent(ctx, run.Project, s.now(), actor, reason, eventType, run.TicketID, payload)
		if err != nil {
			if artifactTarget != "" {
				_ = os.Remove(artifactTarget)
			}
			return contracts.EvidenceItem{}, err
		}
		if err := s.commitMutation(ctx, "add evidence", "evidence", event, func(ctx context.Context) error {
			if err := s.Evidence.SaveEvidence(ctx, evidence); err != nil {
				if artifactTarget != "" {
					_ = os.Remove(artifactTarget)
				}
				return err
			}
			if err := s.Runs.SaveRun(ctx, run); err != nil {
				_ = os.Remove(storage.EvidenceFile(s.Root, run.RunID, evidence.EvidenceID))
				if artifactTarget != "" {
					_ = os.Remove(artifactTarget)
				}
				return err
			}
			return nil
		}); err != nil {
			return contracts.EvidenceItem{}, err
		}
		return evidence, nil
	})
}

func (s *ActionService) CreateHandoff(ctx context.Context, runID string, actor contracts.Actor, reason string, openQuestions []string, risks []string, nextActor string, nextGate contracts.GateKind, nextStatus contracts.Status) (contracts.HandoffPacket, error) {
	return withWriteLock(ctx, s.LockManager, "create handoff", func(ctx context.Context) (contracts.HandoffPacket, error) {
		if !actor.IsValid() {
			return contracts.HandoffPacket{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("invalid actor: %s", actor))
		}
		run, err := s.Runs.LoadRun(ctx, runID)
		if err != nil {
			return contracts.HandoffPacket{}, err
		}
		ticket, err := s.Tickets.GetTicket(ctx, run.TicketID)
		if err != nil {
			return contracts.HandoffPacket{}, err
		}
		evidence, err := s.Evidence.ListEvidence(ctx, runID)
		if err != nil {
			return contracts.HandoffPacket{}, err
		}
		packet, err := buildHandoffPacket(ctx, s.Root, run, ticket, evidence, actor, s.now(), openQuestions, risks, nextActor, nextGate, nextStatus)
		if err != nil {
			return contracts.HandoffPacket{}, err
		}
		ticket.LatestHandoffID = packet.HandoffID
		run.HandoffTo = strings.TrimSpace(nextActor)
		payload := runMutationPayload{Run: run, Ticket: ticket, Handoff: packet}
		event, err := s.newEvent(ctx, run.Project, s.now(), actor, reason, contracts.EventRunHandoffRequested, run.TicketID, payload)
		if err != nil {
			return contracts.HandoffPacket{}, err
		}
		if err := s.commitMutation(ctx, "create handoff", "handoff", event, func(ctx context.Context) error {
			if err := s.Handoffs.SaveHandoff(ctx, packet); err != nil {
				return err
			}
			if err := s.Runs.SaveRun(ctx, run); err != nil {
				_ = os.Remove(storage.HandoffFile(s.Root, packet.HandoffID))
				return err
			}
			if err := s.UpdateTicket(ctx, ticket); err != nil {
				_ = os.Remove(storage.HandoffFile(s.Root, packet.HandoffID))
				return err
			}
			return nil
		}); err != nil {
			return contracts.HandoffPacket{}, err
		}
		return packet, nil
	})
}

func copyEvidenceArtifact(root string, runID string, evidenceID string, source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", fmt.Errorf("stat artifact: %w", err)
	}
	if info.Size() > maxEvidenceArtifactBytes {
		return "", apperr.New(apperr.CodeInvalidInput, "artifact exceeds 10 MiB limit")
	}
	ext := filepath.Ext(source)
	base := slugify(strings.TrimSuffix(filepath.Base(source), ext))
	if base == "" {
		base = evidenceID
	}
	target := filepath.Join(storage.EvidenceDir(root, runID), base+ext)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return "", fmt.Errorf("create artifact copy: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", fmt.Errorf("copy artifact: %w", err)
	}
	return target, nil
}

func buildHandoffPacket(ctx context.Context, root string, run contracts.RunSnapshot, ticket contracts.TicketSnapshot, evidence []contracts.EvidenceItem, actor contracts.Actor, generatedAt time.Time, openQuestions []string, risks []string, nextActor string, nextGate contracts.GateKind, nextStatus contracts.Status) (contracts.HandoffPacket, error) {
	packet := contracts.HandoffPacket{
		HandoffID:                 "handoff_" + NewOpaqueID(),
		SourceRunID:               run.RunID,
		TicketID:                  ticket.ID,
		Actor:                     actor,
		StatusSummary:             strings.TrimSpace(run.Summary),
		OpenQuestions:             compactStrings(openQuestions),
		Risks:                     compactStrings(risks),
		SuggestedNextActor:        strings.TrimSpace(nextActor),
		SuggestedNextGate:         nextGate,
		SuggestedNextTicketStatus: nextStatus,
		GeneratedAt:               generatedAt.UTC(),
		SchemaVersion:             contracts.CurrentSchemaVersion,
	}
	if packet.GeneratedAt.IsZero() {
		packet.GeneratedAt = timeNowUTC()
	}
	for _, item := range evidence {
		packet.EvidenceLinks = append(packet.EvidenceLinks, item.EvidenceID)
	}
	scm := SCMService{Root: root}
	refs, _ := scm.TicketRefs(ctx, ticket.ID)
	for _, ref := range refs {
		packet.CommitRefs = append(packet.CommitRefs, ref.Hash)
	}
	if strings.TrimSpace(run.WorktreePath) != "" {
		if changed, err := scm.gitOutput(ctx, "-C", run.WorktreePath, "diff", "--name-only", "HEAD"); err == nil {
			packet.ChangedFiles = append(packet.ChangedFiles, compactStrings(strings.Split(strings.TrimSpace(changed), "\n"))...)
		}
		if status, err := scm.gitOutput(ctx, "-C", run.WorktreePath, "status", "--short"); err == nil {
			packet.Tests = append(packet.Tests, compactStrings(strings.Split(strings.TrimSpace(status), "\n"))...)
		}
	}
	if packet.StatusSummary == "" {
		packet.StatusSummary = ticket.Title
	}
	return packet, packet.Validate()
}

func compactStrings(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}
	return items
}
