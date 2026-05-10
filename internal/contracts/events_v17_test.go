package contracts

import "testing"

func TestV17EventFamiliesAreRegistered(t *testing.T) {
	events := []EventType{
		EventKeyGenerated,
		EventKeyImported,
		EventKeyRotated,
		EventKeyRevoked,
		EventTrustBound,
		EventTrustRevoked,
		EventSignatureCreated,
		EventSignatureVerified,
		EventGovernancePackCreated,
		EventGovernancePackApplied,
		EventGovernancePolicyUpdated,
		EventGovernanceOverrideRecorded,
		EventClassificationSet,
		EventRedactionPreviewed,
		EventRedactionExported,
		EventAuditReportCreated,
		EventAuditReportExported,
		EventBackupCreated,
		EventBackupVerified,
		EventBackupRestorePlanned,
		EventBackupRestored,
		EventGoalManifestGenerated,
	}
	for _, eventType := range events {
		if !eventType.IsValid() {
			t.Fatalf("expected v1.7 event type %q to be registered", eventType)
		}
	}
}
