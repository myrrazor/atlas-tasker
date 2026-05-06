package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

type OperationApproval struct {
	ID                 string    `json:"id"`
	Operation          string    `json:"operation"`
	Target             string    `json:"target"`
	Actor              string    `json:"actor"`
	Reason             string    `json:"reason"`
	CreatedBySurface   string    `json:"created_by_surface"`
	CreatedAt          time.Time `json:"created_at"`
	ExpiresAt          time.Time `json:"expires_at"`
	UsedAt             time.Time `json:"used_at,omitempty"`
	ConsumedByTool     string    `json:"consumed_by_tool,omitempty"`
	ConsumedByMutation string    `json:"consumed_by_mutation,omitempty"`
}

type ApprovalStore struct {
	Root string
	Now  func() time.Time
}

func NewApprovalStore(root string, now func() time.Time) ApprovalStore {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return ApprovalStore{Root: root, Now: now}
}

func (s ApprovalStore) Create(ctx context.Context, operation string, target string, actor string, ttl time.Duration, reason string) (OperationApproval, error) {
	operation = NormalizeOperation(operation)
	target = strings.TrimSpace(target)
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if operation == "" {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "operation is required")
	}
	if target == "" {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "target is required")
	}
	if actor == "" {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "actor is required")
	}
	if reason == "" {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "reason is required")
	}
	if ttl <= 0 {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "ttl must be positive")
	}
	if ttl > 24*time.Hour {
		return OperationApproval{}, apperr.New(apperr.CodeInvalidInput, "ttl must not exceed 24h")
	}
	now := s.Now().UTC()
	approval := OperationApproval{
		ID:               "mcp_approval_" + randomApprovalID(),
		Operation:        operation,
		Target:           target,
		Actor:            actor,
		Reason:           reason,
		CreatedBySurface: "cli",
		CreatedAt:        now,
		ExpiresAt:        now.Add(ttl).UTC(),
	}
	if err := s.withLock(ctx, "create mcp operation approval", func() error {
		if err := os.MkdirAll(s.dir(), 0o755); err != nil {
			return err
		}
		return s.write(approval)
	}); err != nil {
		return OperationApproval{}, err
	}
	return approval, nil
}

func (s ApprovalStore) Consume(ctx context.Context, approvalID string, operation string, target string, actor string, toolName string) (OperationApproval, error) {
	approvalID = strings.TrimSpace(approvalID)
	operation = NormalizeOperation(operation)
	target = strings.TrimSpace(target)
	actor = strings.TrimSpace(actor)
	toolName = strings.TrimSpace(toolName)
	if approvalID == "" {
		return OperationApproval{}, apperr.New(apperr.CodePermissionDenied, "operation approval id is required")
	}
	var consumed OperationApproval
	err := s.withLock(ctx, "consume mcp operation approval", func() error {
		approval, err := s.load(approvalID)
		if err != nil {
			return err
		}
		if approval.CreatedBySurface == "mcp" {
			return apperr.New(apperr.CodePermissionDenied, "operation approval was created by MCP")
		}
		if !approval.UsedAt.IsZero() {
			return apperr.New(apperr.CodePermissionDenied, "operation approval was already used")
		}
		now := s.Now().UTC()
		if !approval.ExpiresAt.IsZero() && now.After(approval.ExpiresAt) {
			return apperr.New(apperr.CodePermissionDenied, "operation approval expired")
		}
		if approval.Operation != operation {
			return apperr.New(apperr.CodePermissionDenied, "operation approval does not match operation")
		}
		if approval.Target != target {
			return apperr.New(apperr.CodePermissionDenied, "operation approval does not match target")
		}
		if approval.Actor != actor {
			return apperr.New(apperr.CodePermissionDenied, "operation approval does not match actor")
		}
		approval.UsedAt = now
		approval.ConsumedByTool = toolName
		consumed = approval
		return s.write(approval)
	})
	if err != nil {
		return OperationApproval{}, err
	}
	return consumed, nil
}

func (s ApprovalStore) List() ([]OperationApproval, error) {
	entries, err := os.ReadDir(s.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return []OperationApproval{}, nil
		}
		return nil, err
	}
	approvals := []OperationApproval{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		approval, err := s.load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	return approvals, nil
}

func (s ApprovalStore) Revoke(ctx context.Context, approvalID string) error {
	return s.withLock(ctx, "revoke mcp operation approval", func() error {
		if err := os.Remove(s.path(approvalID)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	})
}

func NormalizeOperation(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "atlas.") {
		return value
	}
	return "atlas." + value
}

func (s ApprovalStore) dir() string {
	return filepath.Join(storage.TrackerDir(s.Root), "runtime", "mcp", "operation-approvals")
}

func (s ApprovalStore) path(id string) string {
	id = filepath.Base(strings.TrimSpace(id))
	return filepath.Join(s.dir(), id+".json")
}

func (s ApprovalStore) load(id string) (OperationApproval, error) {
	raw, err := os.ReadFile(s.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			return OperationApproval{}, apperr.New(apperr.CodePermissionDenied, "operation approval not found")
		}
		return OperationApproval{}, err
	}
	var approval OperationApproval
	if err := json.Unmarshal(raw, &approval); err != nil {
		return OperationApproval{}, err
	}
	return approval, nil
}

func (s ApprovalStore) write(approval OperationApproval) error {
	raw, err := json.MarshalIndent(approval, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(approval.ID), append(raw, '\n'), 0o600)
}

func (s ApprovalStore) withLock(ctx context.Context, purpose string, fn func() error) error {
	return service.WithWriteLock(ctx, service.FileLockManager{Root: s.Root}, purpose, func(context.Context) error {
		return fn()
	})
}

func randomApprovalID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
