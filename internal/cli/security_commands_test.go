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

func TestV17GovernanceCLIAndProtectedCompletion(t *testing.T) {
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
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Governed", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_review", "--actor", "human:owner")
	must("ticket", "approve", "APP-1", "--actor", "human:owner")

	createdRaw := must(
		"governance", "pack", "create", "Release quorum",
		"--scope", "project:APP",
		"--protected-action", "ticket_complete",
		"--quorum-count", "1",
		"--actor", "human:owner",
		"--reason", "protect release completion",
		"--json",
	)
	var created struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Pack          struct {
			PackID   string `json:"pack_id"`
			Policies []struct {
				ScopeKind          string   `json:"scope_kind"`
				ScopeID            string   `json:"scope_id"`
				ProtectedActions   []string `json:"protected_actions"`
				RequiredSignatures int      `json:"required_signatures"`
				QuorumRules        []struct {
					RequiredCount int `json:"required_count"`
				} `json:"quorum_rules"`
			} `json:"policies"`
		} `json:"pack"`
	}
	if err := json.Unmarshal([]byte(createdRaw), &created); err != nil {
		t.Fatalf("parse governance pack create: %v\n%s", err, createdRaw)
	}
	if created.FormatVersion != jsonFormatVersion || created.Kind != "governance_pack_detail" || created.Pack.PackID != "release-quorum" || len(created.Pack.Policies) != 1 {
		t.Fatalf("unexpected governance pack create payload: %#v", created)
	}
	if created.Pack.Policies[0].ScopeKind != "project" || created.Pack.Policies[0].ScopeID != "APP" || len(created.Pack.Policies[0].QuorumRules) != 1 || created.Pack.Policies[0].QuorumRules[0].RequiredCount != 1 {
		t.Fatalf("unexpected governance policy payload: %#v", created.Pack.Policies[0])
	}
	must("governance", "pack", "apply", created.Pack.PackID, "--scope", "project:APP", "--actor", "human:owner", "--reason", "enable release policy")

	validateRaw := must("governance", "validate", "--json")
	var validation struct {
		Kind     string `json:"kind"`
		Valid    bool   `json:"valid"`
		Policies int    `json:"policies"`
		Packs    int    `json:"packs"`
	}
	if err := json.Unmarshal([]byte(validateRaw), &validation); err != nil {
		t.Fatalf("parse governance validate: %v\n%s", err, validateRaw)
	}
	if validation.Kind != "governance_validation_result" || !validation.Valid || validation.Policies != 1 || validation.Packs != 1 {
		t.Fatalf("unexpected governance validation: %#v", validation)
	}

	blockedRaw := must("governance", "simulate", "ticket_complete", "--ticket", "APP-1", "--actor", "human:owner", "--json")
	var blocked struct {
		Explanation struct {
			Allowed     bool     `json:"allowed"`
			ReasonCodes []string `json:"reason_codes"`
		} `json:"explanation"`
	}
	if err := json.Unmarshal([]byte(blockedRaw), &blocked); err != nil {
		t.Fatalf("parse governance simulate: %v\n%s", err, blockedRaw)
	}
	if blocked.Explanation.Allowed || !strings.Contains(strings.Join(blocked.Explanation.ReasonCodes, ","), "quorum_unsatisfied") {
		t.Fatalf("missing approval should block simulated completion: %#v", blocked.Explanation)
	}
	allowedRaw := must("governance", "simulate", "ticket_complete", "--ticket", "APP-1", "--actor", "human:owner", "--approval-actor", "human:owner", "--json")
	if !strings.Contains(allowedRaw, `"allowed": true`) && !strings.Contains(allowedRaw, `"allowed":true`) {
		t.Fatalf("approval actor should allow simulation:\n%s", allowedRaw)
	}
	if out, err := runCLI(t, "ticket", "complete", "APP-1", "--actor", "human:owner", "--reason", "ship"); err == nil || !strings.Contains(out+err.Error(), "quorum_unsatisfied") {
		t.Fatalf("protected ticket complete should enforce governance, err=%v out=%s", err, out)
	}
}

func TestV17GovernanceOwnerOverrideIsEvented(t *testing.T) {
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
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Override", "--type", "task", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "ready", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_progress", "--actor", "human:owner")
	must("ticket", "move", "APP-1", "in_review", "--actor", "human:owner")
	must("ticket", "approve", "APP-1", "--actor", "human:owner")
	packRaw := must(
		"governance", "pack", "create", "Release quorum",
		"--scope", "project:APP",
		"--protected-action", "ticket_complete",
		"--quorum-count", "2",
		"--allow-owner-override",
		"--actor", "human:owner",
		"--reason", "protect owner override",
		"--json",
	)
	var pack struct {
		Pack struct {
			PackID string `json:"pack_id"`
		} `json:"pack"`
	}
	if err := json.Unmarshal([]byte(packRaw), &pack); err != nil {
		t.Fatalf("parse override pack: %v\n%s", err, packRaw)
	}
	must("governance", "pack", "apply", pack.Pack.PackID, "--scope", "project:APP", "--actor", "human:owner", "--reason", "enable override policy")
	overrideRaw := must("governance", "simulate", "ticket_complete", "--ticket", "APP-1", "--actor", "human:owner", "--reason", "emergency release override", "--json")
	if !strings.Contains(overrideRaw, `"allowed": true`) && !strings.Contains(overrideRaw, `"allowed":true`) {
		t.Fatalf("governance simulate should model reason-backed owner override:\n%s", overrideRaw)
	}
	if !strings.Contains(overrideRaw, "owner_override_applied") {
		t.Fatalf("governance simulate should report owner override:\n%s", overrideRaw)
	}
	must("ticket", "complete", "APP-1", "--actor", "human:owner", "--reason", "emergency release override")
	history := must("ticket", "history", "APP-1", "--json")
	if !strings.Contains(history, "governance.override.recorded") {
		t.Fatalf("owner override should be evented in ticket history:\n%s", history)
	}
}

func TestV17GovernanceValidateFailsForInvalidPolicy(t *testing.T) {
	withTempWorkspace(t)
	if out, err := runCLI(t, "init"); err != nil {
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if err := os.MkdirAll(storage.GovernancePoliciesDir("."), 0o755); err != nil {
		t.Fatalf("mkdir governance policies: %v", err)
	}
	raw := []byte(`
policy_id = "invalid-ticket-signature"
name = "Invalid ticket signature"
scope_kind = "workspace"
protected_actions = ["ticket_complete"]
required_signatures = 1
schema_version = 1
`)
	if err := os.WriteFile(filepath.Join(storage.GovernancePoliciesDir("."), "invalid-ticket-signature.toml"), raw, 0o644); err != nil {
		t.Fatalf("write invalid governance policy: %v", err)
	}
	out, err := runCLI(t, "governance", "validate", "--json")
	if err == nil {
		t.Fatalf("invalid governance should fail validation:\n%s", out)
	}
	if !strings.Contains(out, `"valid": false`) && !strings.Contains(out, `"valid":false`) {
		t.Fatalf("invalid governance output should include valid=false:\n%s", out)
	}
	if !strings.Contains(out+err.Error(), "required_signatures is not supported") {
		t.Fatalf("validation output should explain the invalid policy, err=%v out=%s", err, out)
	}
}

func TestV17ClassifyAndRedactCLI(t *testing.T) {
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
	must("project", "create", "APP", "App Project")
	must("ticket", "create", "--project", "APP", "--title", "Public", "--type", "task", "--description", "PUBLIC-CLI-CONTENT", "--actor", "human:owner")
	must("ticket", "create", "--project", "APP", "--title", "Restricted", "--type", "task", "--description", "SECRET-CLI-REDACT", "--actor", "human:owner")

	classifiedRaw := must("classify", "set", "ticket:APP-2", "restricted", "--actor", "human:owner", "--reason", "cli restricted label", "--json")
	var classified struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Level         string `json:"level"`
		EntityKind    string `json:"entity_kind"`
		EntityID      string `json:"entity_id"`
	}
	if err := json.Unmarshal([]byte(classifiedRaw), &classified); err != nil {
		t.Fatalf("parse classify set: %v\n%s", err, classifiedRaw)
	}
	if classified.FormatVersion != jsonFormatVersion || classified.Kind != "classification_detail" || classified.Level != "restricted" || classified.EntityID != "APP-2" {
		t.Fatalf("unexpected classification detail: %#v", classified)
	}
	listRaw := must("classify", "list", "--project", "APP", "--json")
	if !strings.Contains(listRaw, `"entity_id": "APP-2"`) && !strings.Contains(listRaw, `"entity_id":"APP-2"`) {
		t.Fatalf("classification list should include APP-2 label:\n%s", listRaw)
	}

	previewRaw := must("redact", "preview", "--scope", "workspace", "--target", "export", "--actor", "human:owner", "--reason", "preview cli redaction", "--json")
	var preview struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Preview       struct {
			PreviewID string `json:"preview_id"`
			Items     []struct {
				EntityID  string `json:"entity_id"`
				FieldPath string `json:"field_path"`
				Action    string `json:"action"`
			} `json:"items"`
		} `json:"preview"`
	}
	if err := json.Unmarshal([]byte(previewRaw), &preview); err != nil {
		t.Fatalf("parse redaction preview: %v\n%s", err, previewRaw)
	}
	if preview.FormatVersion != jsonFormatVersion || preview.Kind != "redaction_preview" || preview.Preview.PreviewID == "" || len(preview.Preview.Items) == 0 {
		t.Fatalf("unexpected redaction preview: %#v", preview)
	}

	exportRaw := must("redact", "export", "--scope", "workspace", "--preview-id", preview.Preview.PreviewID, "--actor", "human:owner", "--reason", "create cli redacted export", "--json")
	var redacted struct {
		FormatVersion string `json:"format_version"`
		Kind          string `json:"kind"`
		Bundle        struct {
			BundleID           string `json:"bundle_id"`
			RedactionPreviewID string `json:"redaction_preview_id"`
		} `json:"bundle"`
		Omitted int `json:"omitted"`
	}
	if err := json.Unmarshal([]byte(exportRaw), &redacted); err != nil {
		t.Fatalf("parse redacted export: %v\n%s", err, exportRaw)
	}
	if redacted.FormatVersion != jsonFormatVersion || redacted.Kind != "redaction_export_result" || redacted.Bundle.BundleID == "" || redacted.Bundle.RedactionPreviewID != preview.Preview.PreviewID || redacted.Omitted == 0 {
		t.Fatalf("unexpected redacted export: %#v", redacted)
	}
	verifyRaw := must("redact", "verify", redacted.Bundle.BundleID, "--json")
	if !strings.Contains(verifyRaw, `"verified": true`) && !strings.Contains(verifyRaw, `"verified":true`) {
		t.Fatalf("redacted export should verify:\n%s", verifyRaw)
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
	cmd.SetArgs([]string{"backup", "create", "--actor", "human:owner"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "non-empty --reason") {
		t.Fatalf("expected missing reason error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backup", "create", "--actor", "human:owner", "--reason", "   "})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "non-empty --reason") {
		t.Fatalf("expected blank reason error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backup", "create", "--actor", "robot:mallory", "--reason", "contract smoke"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "valid --actor") {
		t.Fatalf("expected invalid actor error, got %v", err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backup", "create", "--actor", "human:owner", "--reason", "contract smoke"})
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
