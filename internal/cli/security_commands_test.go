package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestV17ReadStubJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"trust", "status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("trust status stub should succeed: %v\n%s", err, out.String())
	}
	var got struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		GeneratedAt   string `json:"generated_at"`
		Warnings      []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v\n%s", err, out.String())
	}
	if got.FormatVersion != jsonFormatVersion || got.Kind != "trust_status" || got.GeneratedAt == "" {
		t.Fatalf("unexpected v1.7 read envelope: %+v", got)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != "v1_7_contract_only" {
		t.Fatalf("expected v1.7 contract-only warning: %+v", got.Warnings)
	}
}

func TestV17PendingVerificationReadsFailClosed(t *testing.T) {
	for _, args := range [][]string{
		{"verify", "backup", "no-such-file"},
		{"governance", "validate"},
		{"backup", "verify", "backup_1"},
		{"backup", "drill"},
	} {
		cmd := NewRootCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "frozen for v1.7 follow-up implementation") {
			t.Fatalf("%v should fail closed while pending, got %v\n%s", args, err, out.String())
		}
		if strings.TrimSpace(out.String()) != "" {
			t.Fatalf("%v should not write success-shaped output while failing closed:\n%s", args, out.String())
		}
	}
}

func TestV17FailClosedExecuteWritesOnlyErrorEnvelope(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"verify", "backup", "no-such-file", "--json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("expected fail-closed verify command to exit non-zero")
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected no stdout on fail-closed verify command, got:\n%s", stdout.String())
	}
	var envelope struct {
		FormatVersion string `json:"format_version"`
		OK            bool   `json:"ok"`
		Error         struct {
			Code string `json:"code"`
			Exit int    `json:"exit"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v\n%s", err, stderr.String())
	}
	if envelope.FormatVersion != jsonFormatVersion || envelope.OK || envelope.Error.Code != "internal" || envelope.Error.Exit != exit {
		t.Fatalf("unexpected fail-closed envelope: %+v", envelope)
	}
}

func TestV17MutationStubRequiresReasonAndFailsNonZero(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"key", "generate", "--actor", "human:owner"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "non-empty --reason") {
		t.Fatalf("expected missing reason error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"key", "generate", "--actor", "human:owner", "--reason", "   "})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "non-empty --reason") {
		t.Fatalf("expected blank reason error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"key", "generate", "--actor", "robot:mallory", "--reason", "contract smoke"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "valid --actor") {
		t.Fatalf("expected invalid actor error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"key", "generate", "--actor", "human:owner", "--reason", "contract smoke"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "frozen for v1.7 follow-up implementation") {
		t.Fatalf("expected non-zero implementation-pending error, got %v", err)
	}
}
