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

type ChangeStore interface {
	SaveChange(ctx context.Context, change ChangeRef) error
	LoadChange(ctx context.Context, changeID string) (ChangeRef, error)
	ListChanges(ctx context.Context, ticketID string) ([]ChangeRef, error)
}

type CheckStore interface {
	SaveCheck(ctx context.Context, check CheckResult) error
	LoadCheck(ctx context.Context, checkID string) (CheckResult, error)
	ListChecks(ctx context.Context, scope CheckScope, scopeID string) ([]CheckResult, error)
}

type PermissionProfileStore interface {
	SavePermissionProfile(ctx context.Context, profile PermissionProfile) error
	LoadPermissionProfile(ctx context.Context, profileID string) (PermissionProfile, error)
	ListPermissionProfiles(ctx context.Context) ([]PermissionProfile, error)
}

type ImportJobStore interface {
	SaveImportJob(ctx context.Context, job ImportJob) error
	LoadImportJob(ctx context.Context, jobID string) (ImportJob, error)
	ListImportJobs(ctx context.Context) ([]ImportJob, error)
}

type ExportBundleStore interface {
	SaveExportBundle(ctx context.Context, bundle ExportBundle) error
	LoadExportBundle(ctx context.Context, bundleID string) (ExportBundle, error)
	ListExportBundles(ctx context.Context) ([]ExportBundle, error)
}

type RetentionPolicyStore interface {
	SaveRetentionPolicy(ctx context.Context, policy RetentionPolicy) error
	LoadRetentionPolicy(ctx context.Context, policyID string) (RetentionPolicy, error)
	ListRetentionPolicies(ctx context.Context) ([]RetentionPolicy, error)
}

type ArchiveRecordStore interface {
	SaveArchiveRecord(ctx context.Context, record ArchiveRecord) error
	LoadArchiveRecord(ctx context.Context, archiveID string) (ArchiveRecord, error)
	ListArchiveRecords(ctx context.Context) ([]ArchiveRecord, error)
}

type SyncRemoteStore interface {
	SaveSyncRemote(ctx context.Context, remote SyncRemote) error
	LoadSyncRemote(ctx context.Context, remoteID string) (SyncRemote, error)
	ListSyncRemotes(ctx context.Context) ([]SyncRemote, error)
}

type SyncJobStore interface {
	SaveSyncJob(ctx context.Context, job SyncJob) error
	LoadSyncJob(ctx context.Context, jobID string) (SyncJob, error)
	ListSyncJobs(ctx context.Context, remoteID string) ([]SyncJob, error)
}

type ConflictStore interface {
	SaveConflict(ctx context.Context, conflict ConflictRecord) error
	LoadConflict(ctx context.Context, conflictID string) (ConflictRecord, error)
	ListConflicts(ctx context.Context) ([]ConflictRecord, error)
}
