package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var v16Namespace = uuid.MustParse("5dc51fbe-8071-4b98-a2eb-5bfe3bc5b6f7")

func DeterministicUID(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		clean = append(clean, strings.TrimSpace(part))
	}
	return uuid.NewSHA1(v16Namespace, []byte(strings.Join(clean, ":"))).String()
}

func TicketUID(projectKey string, ticketID string) string {
	if strings.TrimSpace(projectKey) == "" || strings.TrimSpace(ticketID) == "" {
		return ""
	}
	return DeterministicUID("ticket", projectKey, ticketID)
}

func RunUID(runID string) string {
	if strings.TrimSpace(runID) == "" {
		return ""
	}
	return DeterministicUID("run", runID)
}

func GateUID(gateID string) string {
	if strings.TrimSpace(gateID) == "" {
		return ""
	}
	return DeterministicUID("gate", gateID)
}

func HandoffUID(handoffID string) string {
	if strings.TrimSpace(handoffID) == "" {
		return ""
	}
	return DeterministicUID("handoff", handoffID)
}

func EvidenceUID(runID string, evidenceID string) string {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(evidenceID) == "" {
		return ""
	}
	return DeterministicUID("evidence", runID, evidenceID)
}

func ChangeUID(changeID string) string {
	if strings.TrimSpace(changeID) == "" {
		return ""
	}
	return DeterministicUID("change", changeID)
}

func CheckUID(checkID string) string {
	if strings.TrimSpace(checkID) == "" {
		return ""
	}
	return DeterministicUID("check", checkID)
}

func MembershipUID(collaboratorID string, scopeKind MembershipScopeKind, scopeID string, role MembershipRole) string {
	if strings.TrimSpace(collaboratorID) == "" || strings.TrimSpace(scopeID) == "" || scopeKind == "" || role == "" {
		return ""
	}
	return DeterministicUID("membership", collaboratorID, string(scopeKind), scopeID, string(role))
}

func MentionUID(sourceEventUID string, collaboratorID string, ordinal int) string {
	if strings.TrimSpace(sourceEventUID) == "" || strings.TrimSpace(collaboratorID) == "" || ordinal < 0 {
		return ""
	}
	return DeterministicUID("mention", sourceEventUID, collaboratorID, fmt.Sprintf("%d", ordinal))
}

func ImportJobUID(jobID string) string {
	if strings.TrimSpace(jobID) == "" {
		return ""
	}
	return DeterministicUID("import", jobID)
}

func ExportBundleUID(bundleID string) string {
	if strings.TrimSpace(bundleID) == "" {
		return ""
	}
	return DeterministicUID("export", bundleID)
}

func ArchiveRecordUID(archiveID string) string {
	if strings.TrimSpace(archiveID) == "" {
		return ""
	}
	return DeterministicUID("archive", archiveID)
}

func LegacyEventUID(event Event) string {
	type digestEvent struct {
		EventID       int64         `json:"event_id"`
		Timestamp     string        `json:"timestamp"`
		Actor         Actor         `json:"actor"`
		Reason        string        `json:"reason,omitempty"`
		Type          EventType     `json:"type"`
		Project       string        `json:"project"`
		TicketID      string        `json:"ticket_id,omitempty"`
		Payload       any           `json:"payload,omitempty"`
		Metadata      EventMetadata `json:"metadata,omitempty"`
		SchemaVersion int           `json:"schema_version"`
	}
	normalized := digestEvent{
		EventID:       event.EventID,
		Timestamp:     event.Timestamp.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		Actor:         event.Actor,
		Reason:        strings.TrimSpace(event.Reason),
		Type:          event.Type,
		Project:       strings.TrimSpace(event.Project),
		TicketID:      strings.TrimSpace(event.TicketID),
		Payload:       event.Payload,
		Metadata:      event.Metadata,
		SchemaVersion: event.SchemaVersion,
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s:%s:%s:%s", event.EventID, normalized.Timestamp, event.Actor, event.Type, event.Project, event.TicketID)))
		return DeterministicUID("event", hex.EncodeToString(hash[:]))
	}
	hash := sha256.Sum256(raw)
	return DeterministicUID("event", hex.EncodeToString(hash[:]))
}
