package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestV17ReadStubJSONEnvelope(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"admin", "security-status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("admin security status stub should succeed: %v\n%s", err, out.String())
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
	if got.FormatVersion != jsonFormatVersion || got.Kind != "admin_security_status" || got.GeneratedAt == "" {
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
	cmd.SetArgs([]string{"classify", "set", "ticket:APP-1", "internal", "--actor", "human:owner"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "non-empty --reason") {
		t.Fatalf("expected missing reason error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"classify", "set", "ticket:APP-1", "internal", "--actor", "human:owner", "--reason", "   "})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "non-empty --reason") {
		t.Fatalf("expected blank reason error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"classify", "set", "ticket:APP-1", "internal", "--actor", "robot:mallory", "--reason", "contract smoke"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "valid --actor") {
		t.Fatalf("expected invalid actor error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"classify", "set", "ticket:APP-1", "internal", "--actor", "human:owner", "--reason", "contract smoke"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "frozen for v1.7 follow-up implementation") {
		t.Fatalf("expected non-zero implementation-pending error, got %v", err)
	}
}

func TestSignatureVerifyOutputIncludesIntegrityFailures(t *testing.T) {
	view := service.ArtifactSignatureVerifyView{
		Kind: "signature_verify_result",
		Integrity: service.ExportVerifyView{
			Verified: false,
			Errors:   []string{"archive_checksum_mismatch"},
		},
		Signature: contracts.SignatureVerificationResult{
			State:        contracts.VerificationTrustedValid,
			ArtifactKind: contracts.ArtifactKindBundle,
			ArtifactUID:  "bundle_1",
		},
	}

	pretty := signatureVerifyPretty(view)
	if !strings.Contains(pretty, "trusted_valid") || !strings.Contains(pretty, "integrity=false") || !strings.Contains(pretty, "archive_checksum_mismatch") {
		t.Fatalf("pretty verify output should expose integrity failure, got %q", pretty)
	}
	md := signatureVerifyMarkdown(view)
	if !strings.Contains(md, "Integrity verified: `false`") || !strings.Contains(md, "Integrity error: `archive_checksum_mismatch`") {
		t.Fatalf("markdown verify output should expose integrity failure:\n%s", md)
	}
}

func TestV17KeyAndTrustCLI(t *testing.T) {
	withTempWorkspace(t)
	must := func(args ...string) string {
		t.Helper()
		out, err := runCLI(t, args...)
		if err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
		return out
	}
	must("init")

	generatedRaw := must("key", "generate", "--scope", "collaborator", "--owner-id", "alice", "--actor", "human:owner", "--reason", "create signing key", "--json")
	if strings.Contains(generatedRaw, "private_key_material") {
		t.Fatalf("key generate output leaked private material:\n%s", generatedRaw)
	}
	var generated struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		PublicKey     struct {
			PublicKeyID       string `json:"public_key_id"`
			PublicKeyMaterial string `json:"public_key_material"`
			Status            string `json:"status"`
			Source            string `json:"source"`
		} `json:"public_key"`
		PrivateKeyHealth struct {
			Present       bool `json:"present"`
			PermissionsOK bool `json:"permissions_ok"`
		} `json:"private_key_health"`
		CanSign bool `json:"can_sign"`
	}
	if err := json.Unmarshal([]byte(generatedRaw), &generated); err != nil {
		t.Fatalf("parse key generate json: %v\n%s", err, generatedRaw)
	}
	if generated.FormatVersion != jsonFormatVersion || generated.Kind != "key_detail" || generated.PublicKey.PublicKeyID == "" || generated.PublicKey.PublicKeyMaterial == "" {
		t.Fatalf("unexpected generated key payload: %#v", generated)
	}
	if generated.PublicKey.Status != "active" || generated.PublicKey.Source != "local" || !generated.PrivateKeyHealth.Present || !generated.PrivateKeyHealth.PermissionsOK || !generated.CanSign {
		t.Fatalf("generated key should be active local signing material: %#v", generated)
	}
	publicExport := must("key", "export-public", generated.PublicKey.PublicKeyID)
	if !strings.HasPrefix(publicExport, "---\n") || !strings.Contains(publicExport, "public_key_material:") {
		t.Fatalf("default public key export should be importable markdown frontmatter:\n%s", publicExport)
	}
	exportPath := filepath.Join(t.TempDir(), "public-key.md")
	if err := os.WriteFile(exportPath, []byte(publicExport), 0o644); err != nil {
		t.Fatalf("write public export fixture: %v", err)
	}
	if out, err := runCLI(t, "key", "import-public", exportPath, "--actor", "human:owner", "--reason", "import same key"); err == nil || !strings.Contains(err.Error()+out, "refusing to overwrite local signing key") {
		t.Fatalf("importing default export should parse before refusing local overwrite, got err=%v out=%s", err, out)
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	privateInfo, err := os.Stat(storage.PrivateKeyFile(root, generated.PublicKey.PublicKeyID))
	if err != nil {
		t.Fatalf("private key file missing: %v", err)
	}
	if runtime.GOOS != "windows" && privateInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %04o, want 0600", privateInfo.Mode().Perm())
	}

	boundRaw := must("trust", "bind-key", "alice", generated.PublicKey.PublicKeyID, "--actor", "human:owner", "--reason", "trust ceremony", "--json")
	if !strings.Contains(boundRaw, `"trust_level": "trusted"`) && !strings.Contains(boundRaw, `"trust_level":"trusted"`) {
		t.Fatalf("trust bind output should include trusted binding:\n%s", boundRaw)
	}
	statusRaw := must("trust", "status", "--json")
	var status struct {
		FormatVersion     string `json:"format_version"`
		Kind              string `json:"kind"`
		TrustedBindings   int    `json:"trusted_bindings"`
		LocalPrivateKeys  int    `json:"local_private_keys"`
		ImportedUntrusted int    `json:"imported_untrusted"`
	}
	if err := json.Unmarshal([]byte(statusRaw), &status); err != nil {
		t.Fatalf("parse trust status: %v\n%s", err, statusRaw)
	}
	if status.FormatVersion != jsonFormatVersion || status.Kind != "trust_status" || status.TrustedBindings != 1 || status.LocalPrivateKeys != 1 || status.ImportedUntrusted != 0 {
		t.Fatalf("unexpected trust status: %#v", status)
	}

	revokedRaw := must("trust", "revoke-key", generated.PublicKey.PublicKeyID, "--actor", "human:owner", "--reason", "rotate ceremony", "--json")
	if !strings.Contains(revokedRaw, `"trust_level": "revoked"`) && !strings.Contains(revokedRaw, `"trust_level":"revoked"`) {
		t.Fatalf("trust revoke output should include revoked binding:\n%s", revokedRaw)
	}
	rotatedRaw := must("key", "rotate", generated.PublicKey.PublicKeyID, "--actor", "human:owner", "--reason", "rotate key", "--json")
	if !strings.Contains(rotatedRaw, `"status": "rotated"`) && !strings.Contains(rotatedRaw, `"status":"rotated"`) {
		t.Fatalf("key rotate output should mark key rotated:\n%s", rotatedRaw)
	}
}
