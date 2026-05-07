package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type SecurityKeyStore struct {
	Root string
}

type SecurityTrustStore struct {
	Root string
}

type SecuritySignatureStore struct {
	Root string
}

type privateKeyFile struct {
	PublicKeyID        string                 `json:"public_key_id"`
	Fingerprint        string                 `json:"fingerprint"`
	Algorithm          contracts.KeyAlgorithm `json:"algorithm"`
	PrivateKeyMaterial string                 `json:"private_key_material"`
	CreatedAt          string                 `json:"created_at"`
	SchemaVersion      int                    `json:"schema_version"`
}

type privateKeyHealth struct {
	PublicKeyID     string   `json:"public_key_id"`
	Present         bool     `json:"present"`
	PermissionsOK   bool     `json:"permissions_ok"`
	Mode            string   `json:"mode,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	UnsupportedMode bool     `json:"unsupported_mode,omitempty"`
}

type publicKeyFrontmatter struct {
	contracts.PublicKeyRecord `yaml:",inline"`
}

type revocationFrontmatter struct {
	contracts.RevocationRecord `yaml:",inline"`
}

func (s SecurityKeyStore) SavePublicKey(_ context.Context, record contracts.PublicKeyRecord) error {
	record = normalizePublicKeyRecord(record)
	var err error
	record, err = bindPublicKeyMaterial(record)
	if err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.PublicKeysDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create public keys dir: %w", err)
	}
	raw, err := yaml.Marshal(publicKeyFrontmatter{PublicKeyRecord: record})
	if err != nil {
		return fmt.Errorf("marshal public key %s: %w", record.PublicKeyID, err)
	}
	body := fmt.Sprintf("Atlas public key `%s` for `%s:%s`.", record.PublicKeyID, record.OwnerKind, record.OwnerID)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.PublicKeyFile(s.Root, sanitizeSecurityID(record.PublicKeyID)), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write public key %s: %w", record.PublicKeyID, err)
	}
	return nil
}

func (s SecurityKeyStore) LoadPublicKey(_ context.Context, publicKeyID string) (contracts.PublicKeyRecord, error) {
	raw, err := os.ReadFile(storage.PublicKeyFile(s.Root, sanitizeSecurityID(publicKeyID)))
	if err != nil {
		return contracts.PublicKeyRecord{}, fmt.Errorf("read public key %s: %w", publicKeyID, err)
	}
	record, err := decodePublicKeyRecord(raw)
	if err != nil {
		return contracts.PublicKeyRecord{}, fmt.Errorf("parse public key %s: %w", publicKeyID, err)
	}
	record = normalizePublicKeyRecord(record)
	record, err = bindPublicKeyMaterial(record)
	if err != nil {
		return contracts.PublicKeyRecord{}, err
	}
	return record, record.Validate()
}

func (s SecurityKeyStore) ImportPublicKeyFile(path string) (contracts.PublicKeyRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contracts.PublicKeyRecord{}, fmt.Errorf("read public key import: %w", err)
	}
	record, err := decodePublicKeyRecord(raw)
	if err != nil {
		return contracts.PublicKeyRecord{}, err
	}
	record = normalizePublicKeyRecord(record)
	record, err = bindPublicKeyMaterial(record)
	if err != nil {
		return contracts.PublicKeyRecord{}, err
	}
	record.Source = contracts.PublicKeySourceManualImport
	if record.Status == "" || record.Status == contracts.KeyStateActive || record.Status == contracts.KeyStateGenerated {
		record.Status = contracts.KeyStateImported
	}
	return record, record.Validate()
}

func (s SecurityKeyStore) ListPublicKeys(ctx context.Context) ([]contracts.PublicKeyRecord, error) {
	entries, err := os.ReadDir(storage.PublicKeysDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.PublicKeyRecord{}, nil
		}
		return nil, fmt.Errorf("read public keys dir: %w", err)
	}
	items := make([]contracts.PublicKeyRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		record, err := s.LoadPublicKey(ctx, strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].PublicKeyID < items[j].PublicKeyID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s SecurityKeyStore) SavePrivateKey(_ context.Context, key privateKeyFile) error {
	if strings.TrimSpace(key.PublicKeyID) == "" || strings.TrimSpace(key.PrivateKeyMaterial) == "" {
		return fmt.Errorf("public_key_id and private key material are required")
	}
	if err := os.MkdirAll(storage.PrivateKeysDir(s.Root), 0o700); err != nil {
		return fmt.Errorf("create private keys dir: %w", err)
	}
	raw, err := json.MarshalIndent(key, "", "  ")
	if err != nil {
		return fmt.Errorf("encode private key metadata: %w", err)
	}
	path := storage.PrivateKeyFile(s.Root, sanitizeSecurityID(key.PublicKeyID))
	if err := writeFileAtomic(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write private key %s: %w", key.PublicKeyID, err)
	}
	return nil
}

func (s SecurityKeyStore) LoadPrivateKey(_ context.Context, publicKeyID string) (privateKeyFile, error) {
	raw, err := os.ReadFile(storage.PrivateKeyFile(s.Root, sanitizeSecurityID(publicKeyID)))
	if err != nil {
		return privateKeyFile{}, fmt.Errorf("read private key %s: %w", publicKeyID, err)
	}
	var key privateKeyFile
	if err := json.Unmarshal(raw, &key); err != nil {
		return privateKeyFile{}, fmt.Errorf("decode private key %s: %w", publicKeyID, err)
	}
	return key, nil
}

func (s SecurityKeyStore) PrivateKeyHealth(publicKeyID string) privateKeyHealth {
	id := sanitizeSecurityID(publicKeyID)
	path := storage.PrivateKeyFile(s.Root, id)
	health := privateKeyHealth{PublicKeyID: id}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			health.Warnings = append(health.Warnings, "private_key_missing")
			return health
		}
		health.Warnings = append(health.Warnings, "private_key_stat_failed")
		return health
	}
	health.Present = true
	mode := info.Mode().Perm()
	health.Mode = fmt.Sprintf("%04o", mode)
	permissionsOK, unsupported, warnings := evaluatePrivateKeyMode(runtime.GOOS, mode)
	health.PermissionsOK = permissionsOK
	health.UnsupportedMode = unsupported
	health.Warnings = append(health.Warnings, warnings...)
	return health
}

func evaluatePrivateKeyMode(goos string, mode os.FileMode) (bool, bool, []string) {
	if goos == "windows" || goos == "plan9" {
		return true, true, []string{"private_key_permissions_unverified"}
	}
	if mode&0o077 == 0 && mode&0o400 != 0 {
		return true, false, nil
	}
	return false, false, []string{"private_key_permissions_too_broad"}
}

func (s SecurityKeyStore) SaveRevocation(_ context.Context, record contracts.RevocationRecord) error {
	record = normalizeRevocation(record)
	if err := record.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.RevocationsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create revocations dir: %w", err)
	}
	raw, err := yaml.Marshal(revocationFrontmatter{RevocationRecord: record})
	if err != nil {
		return fmt.Errorf("marshal revocation %s: %w", record.RevocationID, err)
	}
	doc := fmt.Sprintf("---\n%s---\n\nRevoked `%s`.\n", string(raw), record.PublicKeyID)
	if err := os.WriteFile(storage.RevocationFile(s.Root, sanitizeSecurityID(record.RevocationID)), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write revocation %s: %w", record.RevocationID, err)
	}
	return nil
}

func (s SecurityKeyStore) ListRevocations(_ context.Context) ([]contracts.RevocationRecord, error) {
	entries, err := os.ReadDir(storage.RevocationsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.RevocationRecord{}, nil
		}
		return nil, fmt.Errorf("read revocations dir: %w", err)
	}
	items := make([]contracts.RevocationRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(storage.RevocationsDir(s.Root), entry.Name()))
		if err != nil {
			return nil, err
		}
		fmRaw, _, err := splitDocument(string(raw))
		if err != nil {
			return nil, err
		}
		var fm revocationFrontmatter
		if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
			return nil, err
		}
		record := normalizeRevocation(fm.RevocationRecord)
		if err := record.Validate(); err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].RevokedAt.Before(items[j].RevokedAt)
	})
	return items, nil
}

func (s SecurityTrustStore) SaveTrustBinding(_ context.Context, binding contracts.TrustBinding) error {
	binding = normalizeTrustBinding(binding)
	if err := binding.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.TrustBindingsDir(s.Root), 0o700); err != nil {
		return fmt.Errorf("create trust dir: %w", err)
	}
	raw, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust binding %s: %w", binding.TrustBindingID, err)
	}
	if err := os.WriteFile(storage.TrustBindingFile(s.Root, sanitizeSecurityID(binding.TrustBindingID)), append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write trust binding %s: %w", binding.TrustBindingID, err)
	}
	return nil
}

func (s SecurityTrustStore) LoadTrustBinding(_ context.Context, trustBindingID string) (contracts.TrustBinding, error) {
	raw, err := os.ReadFile(storage.TrustBindingFile(s.Root, sanitizeSecurityID(trustBindingID)))
	if err != nil {
		return contracts.TrustBinding{}, fmt.Errorf("read trust binding %s: %w", trustBindingID, err)
	}
	var binding contracts.TrustBinding
	if err := json.Unmarshal(raw, &binding); err != nil {
		return contracts.TrustBinding{}, fmt.Errorf("decode trust binding %s: %w", trustBindingID, err)
	}
	binding = normalizeTrustBinding(binding)
	return binding, binding.Validate()
}

func (s SecurityTrustStore) ListTrustBindings(ctx context.Context) ([]contracts.TrustBinding, error) {
	entries, err := os.ReadDir(storage.TrustBindingsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.TrustBinding{}, nil
		}
		return nil, fmt.Errorf("read trust dir: %w", err)
	}
	items := make([]contracts.TrustBinding, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		binding, err := s.LoadTrustBinding(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, binding)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].TrustBindingID < items[j].TrustBindingID
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	return items, nil
}

func (s SecurityTrustStore) ListTrustBindingsForKey(ctx context.Context, publicKeyID string) ([]contracts.TrustBinding, error) {
	all, err := s.ListTrustBindings(ctx)
	if err != nil {
		return nil, err
	}
	filter := strings.TrimSpace(publicKeyID)
	items := make([]contracts.TrustBinding, 0)
	for _, item := range all {
		if item.PublicKeyID == filter {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s SecuritySignatureStore) SaveSignature(_ context.Context, envelope contracts.SignatureEnvelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.SignaturesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create signatures dir: %w", err)
	}
	raw, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("encode signature %s: %w", envelope.SignatureID, err)
	}
	if err := os.WriteFile(storage.SignatureFile(s.Root, sanitizeSecurityID(envelope.SignatureID)), append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write signature %s: %w", envelope.SignatureID, err)
	}
	return nil
}

func (s SecuritySignatureStore) LoadSignature(_ context.Context, signatureID string) (contracts.SignatureEnvelope, error) {
	raw, err := os.ReadFile(storage.SignatureFile(s.Root, sanitizeSecurityID(signatureID)))
	if err != nil {
		return contracts.SignatureEnvelope{}, fmt.Errorf("read signature %s: %w", signatureID, err)
	}
	var envelope contracts.SignatureEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return contracts.SignatureEnvelope{}, fmt.Errorf("decode signature %s: %w", signatureID, err)
	}
	return envelope, envelope.Validate()
}

func (s SecuritySignatureStore) ListSignatures(_ context.Context) ([]contracts.SignatureEnvelope, error) {
	entries, err := os.ReadDir(storage.SignaturesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.SignatureEnvelope{}, nil
		}
		return nil, fmt.Errorf("read signatures dir: %w", err)
	}
	items := make([]contracts.SignatureEnvelope, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		envelope, err := s.LoadSignature(context.Background(), strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		items = append(items, envelope)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SignedAt.Equal(items[j].SignedAt) {
			return items[i].SignatureID < items[j].SignatureID
		}
		return items[i].SignedAt.Before(items[j].SignedAt)
	})
	return items, nil
}

func decodePublicKeyRecord(raw []byte) (contracts.PublicKeyRecord, error) {
	var record contracts.PublicKeyRecord
	if json.Unmarshal(raw, &record) == nil && strings.TrimSpace(record.PublicKeyID) != "" {
		return record, nil
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.PublicKeyRecord{}, err
	}
	var fm publicKeyFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.PublicKeyRecord{}, err
	}
	return fm.PublicKeyRecord, nil
}

func normalizePublicKeyRecord(record contracts.PublicKeyRecord) contracts.PublicKeyRecord {
	record.PublicKeyID = sanitizeSecurityID(record.PublicKeyID)
	record.Fingerprint = strings.TrimSpace(record.Fingerprint)
	record.PublicKeyMaterial = strings.TrimSpace(record.PublicKeyMaterial)
	record.OwnerID = strings.TrimSpace(record.OwnerID)
	if record.Algorithm == "" {
		record.Algorithm = contracts.KeyAlgorithmEd25519
	}
	if record.Status == "" {
		record.Status = contracts.KeyStateImported
	}
	if record.Source == "" {
		record.Source = contracts.PublicKeySourceManualImport
	}
	if record.SchemaVersion == 0 {
		record.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return record
}

func bindPublicKeyMaterial(record contracts.PublicKeyRecord) (contracts.PublicKeyRecord, error) {
	publicBytes, err := base64.StdEncoding.DecodeString(record.PublicKeyMaterial)
	if err != nil {
		return contracts.PublicKeyRecord{}, fmt.Errorf("decode public_key_material: %w", err)
	}
	if len(publicBytes) != ed25519.PublicKeySize {
		return contracts.PublicKeyRecord{}, fmt.Errorf("public_key_material has invalid length")
	}
	derivedFingerprint := fingerprintPublicKey(ed25519.PublicKey(publicBytes))
	if record.Fingerprint == "" {
		record.Fingerprint = derivedFingerprint
	}
	if record.Fingerprint != derivedFingerprint {
		return contracts.PublicKeyRecord{}, fmt.Errorf("public key fingerprint does not match public_key_material")
	}
	derivedID := "key-" + fingerprintShort(derivedFingerprint)
	if record.PublicKeyID == "" {
		record.PublicKeyID = derivedID
	}
	if record.PublicKeyID != derivedID {
		return contracts.PublicKeyRecord{}, fmt.Errorf("public key id does not match public_key_material")
	}
	return record, nil
}

func normalizeRevocation(record contracts.RevocationRecord) contracts.RevocationRecord {
	record.RevocationID = sanitizeSecurityID(record.RevocationID)
	record.PublicKeyID = sanitizeSecurityID(record.PublicKeyID)
	record.Fingerprint = strings.TrimSpace(record.Fingerprint)
	record.Reason = strings.TrimSpace(record.Reason)
	if record.SchemaVersion == 0 {
		record.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return record
}

func normalizeTrustBinding(binding contracts.TrustBinding) contracts.TrustBinding {
	binding.TrustBindingID = sanitizeSecurityID(binding.TrustBindingID)
	binding.PublicKeyID = sanitizeSecurityID(binding.PublicKeyID)
	binding.Fingerprint = strings.TrimSpace(binding.Fingerprint)
	binding.TrustedOwnerID = strings.TrimSpace(binding.TrustedOwnerID)
	binding.Reason = strings.TrimSpace(binding.Reason)
	binding.LocalOnly = true
	if binding.SchemaVersion == 0 {
		binding.SchemaVersion = contracts.CurrentSchemaVersion
	}
	return binding
}

func sanitizeSecurityID(raw string) string {
	id := strings.TrimSpace(strings.ToLower(raw))
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "\\", "-")
	id = strings.ReplaceAll(id, "_", "-")
	if id == "" {
		return "security-record"
	}
	return id
}
