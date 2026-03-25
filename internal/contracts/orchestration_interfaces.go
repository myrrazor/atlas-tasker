package contracts

import "context"

type AgentStore interface {
	SaveAgent(ctx context.Context, profile AgentProfile) error
	LoadAgent(ctx context.Context, agentID string) (AgentProfile, error)
	ListAgents(ctx context.Context) ([]AgentProfile, error)
	DeleteAgent(ctx context.Context, agentID string) error
}

type RunStore interface {
	SaveRun(ctx context.Context, run RunSnapshot) error
	LoadRun(ctx context.Context, runID string) (RunSnapshot, error)
	ListRuns(ctx context.Context, ticketID string) ([]RunSnapshot, error)
}

type RunbookStore interface {
	SaveRunbook(ctx context.Context, runbook Runbook) error
	LoadRunbook(ctx context.Context, name string) (Runbook, error)
	ListRunbooks(ctx context.Context) ([]Runbook, error)
}

type GateStore interface {
	SaveGate(ctx context.Context, gate GateSnapshot) error
	LoadGate(ctx context.Context, gateID string) (GateSnapshot, error)
	ListGates(ctx context.Context, ticketID string) ([]GateSnapshot, error)
}

type EvidenceStore interface {
	SaveEvidence(ctx context.Context, evidence EvidenceItem) error
	LoadEvidence(ctx context.Context, evidenceID string) (EvidenceItem, error)
	ListEvidence(ctx context.Context, runID string) ([]EvidenceItem, error)
}

type HandoffStore interface {
	SaveHandoff(ctx context.Context, handoff HandoffPacket) error
	LoadHandoff(ctx context.Context, handoffID string) (HandoffPacket, error)
	ListHandoffs(ctx context.Context, ticketID string) ([]HandoffPacket, error)
}
