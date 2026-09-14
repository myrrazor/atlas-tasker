package host

import (
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

func TestNativeProofGetEchoIsNotLive(t *testing.T) {
	proves, doctor := nativeProof(integrations.TargetClaude, adapter.VerificationClientCLIGet, true, true, `{"name":"atlas","command":"/usr/local/bin/tracker"}`)
	if proves || doctor {
		t.Fatalf("mcp get echo must not prove a live connection, proves=%v doctor=%v", proves, doctor)
	}
	proves, doctor = nativeProof(integrations.TargetOpenClaw, adapter.VerificationClientCLIDoctor, true, true, "atlas healthy")
	if !proves || !doctor {
		t.Fatalf("doctor still proves a live connection, proves=%v doctor=%v", proves, doctor)
	}
	proves, doctor = nativeProof(integrations.TargetOpenClaw, adapter.VerificationClientCLIDoctor, true, true, "atlas probe failed: connection refused")
	if proves || doctor {
		t.Fatalf("failed doctor must not prove connected, proves=%v doctor=%v", proves, doctor)
	}
}
