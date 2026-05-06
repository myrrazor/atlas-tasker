package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type SecurityAuditRecord struct {
	Timestamp          time.Time   `json:"timestamp"`
	Actor              string      `json:"actor,omitempty"`
	Tool               string      `json:"tool"`
	Target             string      `json:"target,omitempty"`
	ReasonCode         string      `json:"reason_code"`
	Message            string      `json:"message"`
	Profile            ToolProfile `json:"profile"`
	ApprovalID         string      `json:"approval_id,omitempty"`
	ApprovalIDProvided bool        `json:"approval_id_provided,omitempty"`
	HighImpact         bool        `json:"high_impact"`
	ProviderSideEffect bool        `json:"provider_live_side_effect"`
}

func AppendSecurityAudit(root string, record SecurityAuditRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	dir := filepath.Join(storage.TrackerDir(root), "runtime", "mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "security-audit.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(raw, '\n'))
	return err
}
