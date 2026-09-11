package contracts

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	BackupTargetFormat   = "atlas_backup_target_v1"
	BackupTargetTypeGit  = "git"
	BackupVisibilityPriv = "private"
	BackupVisibilityPub  = "public"

	VerificationPolicyExactCommit = "exact_commit"

	BackupErrorOffline                   = "offline"
	BackupErrorDNSFailure                = "dns_failure"
	BackupErrorAuthenticationFailed      = "authentication_failed"
	BackupErrorPermissionDenied          = "permission_denied"
	BackupErrorRemoteMissing             = "remote_missing"
	BackupErrorRemoteDiverged            = "remote_diverged"
	BackupErrorTimeout                   = "timeout"
	BackupErrorCorruptRemoteCheckpoint   = "corrupt_remote_checkpoint"
	BackupErrorRetryableTransportFailure = "retryable_transport_failure"
	BackupErrorHostKeyUnverified         = "host_key_unverified"
	BackupErrorBlockedRemoteDiverged     = "blocked_remote_diverged"
	BackupErrorRemoteRefMissing          = "remote_ref_missing"
)

// BackupTarget is the machine-local atlas_backup_target_v1 record (ADR §4.3).
type BackupTarget struct {
	Format                 string    `json:"format"`
	TargetID               string    `json:"target_id"`
	Type                   string    `json:"type"`
	URL                    string    `json:"url"`
	Enabled                bool      `json:"enabled"`
	DefaultWorkspaceScope  string    `json:"default_workspace_scope,omitempty"`
	VerificationPolicy     string    `json:"verification_policy,omitempty"`
	TransportTimeoutMS     int       `json:"transport_timeout_ms,omitempty"`
	VisibilityAttestation  string    `json:"visibility_attestation"`
	PublicOverrideApproved bool      `json:"public_override_approved,omitempty"`
	DataBoundaryAck        bool      `json:"data_boundary_ack"`
	OriginOverlap          bool      `json:"origin_overlap,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at,omitempty"`
	LastVerified           time.Time `json:"last_verified,omitempty"`
}

func (t BackupTarget) Validate() error {
	if strings.TrimSpace(t.Format) != "" && t.Format != BackupTargetFormat {
		return fmt.Errorf("unsupported backup target format: %s", t.Format)
	}
	if strings.TrimSpace(t.TargetID) == "" {
		return fmt.Errorf("target_id is required")
	}
	if t.Type != "" && t.Type != BackupTargetTypeGit {
		return fmt.Errorf("backup target type must be git")
	}
	if err := ValidateBackupTargetURL(t.URL); err != nil {
		return err
	}
	if !t.DataBoundaryAck {
		return fmt.Errorf("explicit data-boundary confirmation is required")
	}
	if strings.TrimSpace(t.VisibilityAttestation) == "" {
		return fmt.Errorf("visibility attestation is required")
	}
	if t.VisibilityAttestation != BackupVisibilityPriv && t.VisibilityAttestation != BackupVisibilityPub {
		return fmt.Errorf("visibility attestation must be private or public")
	}
	if t.VisibilityAttestation == BackupVisibilityPub && !t.PublicOverrideApproved {
		return fmt.Errorf("public Git repositories require an independent advanced override")
	}
	if t.VisibilityAttestation != BackupVisibilityPriv && IsPublicGitHubHost(t.URL) && !t.PublicOverrideApproved {
		return fmt.Errorf("a confirmed public GitHub repository is refused unless an advanced override is independently approved")
	}
	if t.VerificationPolicy != "" && t.VerificationPolicy != VerificationPolicyExactCommit {
		return fmt.Errorf("verification policy must be exact_commit")
	}
	if t.TransportTimeoutMS < 0 {
		return fmt.Errorf("transport timeout cannot be negative")
	}
	return nil
}

// ValidateBackupTargetURL rejects credential userinfo and unknown schemes.
// Accepted: https, ssh, scp-like SSH, and file:// disposable local remotes.
func ValidateBackupTargetURL(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("backup target URL is required")
	}
	if strings.ContainsAny(value, "\n\r\x00") {
		return fmt.Errorf("backup target URL contains unsafe characters")
	}
	if containsBackupCredentialURL(value) {
		return fmt.Errorf("backup target URL must not embed credentials")
	}
	switch {
	case strings.HasPrefix(value, "https://"):
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid https backup URL")
		}
		if parsed.User != nil {
			return fmt.Errorf("backup target URL must not embed credentials")
		}
		if parsed.Host == "" || parsed.Path == "" || parsed.Path == "/" {
			return fmt.Errorf("https backup URL must include a host and path")
		}
		return nil
	case strings.HasPrefix(value, "ssh://"):
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid ssh backup URL")
		}
		if parsed.User != nil {
			if _, hasPass := parsed.User.Password(); hasPass {
				return fmt.Errorf("backup target URL must not embed credentials")
			}
		}
		if parsed.Host == "" {
			return fmt.Errorf("ssh backup URL must include a host")
		}
		return nil
	case strings.HasPrefix(value, "file://"):
		parsed, err := url.Parse(value)
		if err != nil || parsed.Path == "" || parsed.Path == "/" {
			return fmt.Errorf("file backup URL must name a local path")
		}
		if parsed.User != nil {
			return fmt.Errorf("backup target URL must not embed credentials")
		}
		return nil
	case isSCPLikeGitURL(value):
		return nil
	default:
		return fmt.Errorf("backup target URL must be https, ssh, scp-like SSH, or a local file:// remote")
	}
}

func containsBackupCredentialURL(value string) bool {
	idx := strings.Index(value, "://")
	if idx < 0 {
		return strings.Contains(value, "://")
	}
	rest := value[idx+3:]
	end := strings.IndexAny(rest, "/?#")
	if end >= 0 {
		rest = rest[:end]
	}
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return false
	}
	userinfo := rest[:at]
	return strings.Contains(userinfo, ":")
}

func isSCPLikeGitURL(value string) bool {
	if strings.Contains(value, "://") {
		return false
	}
	at := strings.Index(value, "@")
	colon := strings.LastIndex(value, ":")
	if at <= 0 || colon <= at+1 {
		return false
	}
	host := value[at+1 : colon]
	path := value[colon+1:]
	if host == "" || path == "" || strings.Contains(host, "/") {
		return false
	}
	if strings.Contains(value[:at], ":") {
		return false
	}
	return true
}

// IsPublicGitHubHost reports whether the URL names github.com / gist.github.com.
func IsPublicGitHubHost(raw string) bool {
	host := BackupURLHost(raw)
	host = strings.ToLower(host)
	return host == "github.com" || host == "www.github.com" || host == "gist.github.com"
}

// BackupURLHost returns the host component for diagnostics (never userinfo).
func BackupURLHost(raw string) string {
	value := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "ssh://") || strings.HasPrefix(value, "file://"):
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		return parsed.Host
	case isSCPLikeGitURL(value):
		at := strings.Index(value, "@")
		colon := strings.LastIndex(value, ":")
		if at >= 0 && colon > at {
			return value[at+1 : colon]
		}
	}
	return ""
}

// RedactBackupURL keeps scheme and host only.
func RedactBackupURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "file://") {
		return "file://local"
	}
	if host := BackupURLHost(value); host != "" {
		if strings.HasPrefix(value, "https://") {
			return "https://" + host
		}
		if strings.HasPrefix(value, "ssh://") || isSCPLikeGitURL(value) {
			return "ssh://" + host
		}
	}
	return "redacted"
}

// NormalizeBackupURL makes origin-overlap comparison stable.
func NormalizeBackupURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if isSCPLikeGitURL(value) {
		at := strings.Index(value, "@")
		colon := strings.LastIndex(value, ":")
		user := value[:at]
		host := strings.ToLower(value[at+1 : colon])
		path := strings.TrimSuffix(value[colon+1:], ".git")
		return strings.ToLower(user) + "@" + host + ":" + strings.TrimPrefix(path, "/")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return strings.ToLower(strings.TrimSuffix(value, ".git"))
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimSuffix(parsed.Path, ".git")
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
