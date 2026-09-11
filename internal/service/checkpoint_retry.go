package service

import (
	"math/rand"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const (
	backupRetryInitial = 30 * time.Second
	backupRetryMax     = 30 * time.Minute
)

func classifyRemoteBackupError(err error) string {
	if err == nil {
		return ""
	}
	if apperr.CodeOf(err) == apperr.CodeBusy || strings.Contains(strings.ToLower(err.Error()), "busy") {
		return "busy"
	}
	msg := strings.ToLower(sanitizeGitMessage(err.Error()))
	switch {
	case strings.Contains(msg, "injected checkpoint crash"):
		return "injected_crash"
	case strings.Contains(msg, "host key") || strings.Contains(msg, "known_hosts") || strings.Contains(msg, "remote host identification"):
		return contracts.BackupErrorHostKeyUnverified
	case strings.Contains(msg, "could not resolve") || strings.Contains(msg, "name or service not known") || strings.Contains(msg, "no such host") || strings.Contains(msg, "temporary failure in name resolution"):
		return contracts.BackupErrorDNSFailure
	case strings.Contains(msg, "network is unreachable") || strings.Contains(msg, "no route to host") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "unable to access"):
		return contracts.BackupErrorOffline
	case strings.Contains(msg, "authentication failed") || strings.Contains(msg, "publickey") || strings.Contains(msg, "permission denied (publickey") || strings.Contains(msg, "invalid username or password") || strings.Contains(msg, "401"):
		return contracts.BackupErrorAuthenticationFailed
	case strings.Contains(msg, "permission denied") || strings.Contains(msg, "403") || strings.Contains(msg, "write access"):
		return contracts.BackupErrorPermissionDenied
	case strings.Contains(msg, "not found") || strings.Contains(msg, "does not appear to be a git repository") || strings.Contains(msg, "repository not found"):
		return contracts.BackupErrorRemoteMissing
	case strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "failed to push some refs") || strings.Contains(msg, "rejected") && strings.Contains(msg, "fetch first"):
		return contracts.BackupErrorRemoteDiverged
	case strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "timed out") || strings.Contains(msg, "timeout"):
		return contracts.BackupErrorTimeout
	case strings.Contains(msg, "manifest") || strings.Contains(msg, "corrupt") || strings.Contains(msg, "checksum"):
		return contracts.BackupErrorCorruptRemoteCheckpoint
	case strings.Contains(msg, "diverged"):
		return contracts.BackupErrorBlockedRemoteDiverged
	default:
		if strings.Contains(msg, "git") || strings.Contains(msg, "ssh") || strings.Contains(msg, "https") {
			return contracts.BackupErrorRetryableTransportFailure
		}
		return classifyBackupError(err)
	}
}

func remotePublishErrorClass(class string) bool {
	switch class {
	case contracts.BackupErrorOffline,
		contracts.BackupErrorDNSFailure,
		contracts.BackupErrorAuthenticationFailed,
		contracts.BackupErrorPermissionDenied,
		contracts.BackupErrorRemoteMissing,
		contracts.BackupErrorRemoteDiverged,
		contracts.BackupErrorTimeout,
		contracts.BackupErrorCorruptRemoteCheckpoint,
		contracts.BackupErrorRetryableTransportFailure,
		contracts.BackupErrorHostKeyUnverified,
		contracts.BackupErrorBlockedRemoteDiverged,
		contracts.BackupErrorRemoteRefMissing:
		return true
	default:
		return false
	}
}

func retryableBackupClass(class string) bool {
	switch class {
	case contracts.BackupErrorOffline, contracts.BackupErrorDNSFailure, contracts.BackupErrorTimeout, contracts.BackupErrorRetryableTransportFailure, "busy":
		return true
	case contracts.BackupErrorAuthenticationFailed:
		return true
	default:
		return false
	}
}

func aggressiveRetryClass(class string) bool {
	return class != contracts.BackupErrorAuthenticationFailed && class != contracts.BackupErrorPermissionDenied && class != contracts.BackupErrorHostKeyUnverified
}

func backupRetryWindow(attempt int, class string) time.Duration {
	if !aggressiveRetryClass(class) {
		return backupRetryMax
	}
	delay := backupRetryInitial
	for i := 0; i < attempt; i++ {
		if delay >= backupRetryMax/2 {
			return backupRetryMax
		}
		delay *= 2
	}
	if delay > backupRetryMax {
		return backupRetryMax
	}
	return delay
}

func nextBackupRetry(now time.Time, attempt int, class string) (time.Time, int) {
	if !retryableBackupClass(class) {
		return time.Time{}, attempt
	}
	nextAttempt := attempt + 1
	delay := backupRetryWindow(attempt, class)
	if delay > 0 {
		jitter := time.Duration(rand.Int63n(int64(delay) + 1))
		delay = jitter
	}
	return now.Add(delay), nextAttempt
}

func sanitizeGitMessage(raw string) string {
	out := raw
	replacements := []string{"http://", "https://", "ssh://", "file://", "git@"}
	lower := strings.ToLower(out)
	for _, needle := range replacements {
		for {
			idx := strings.Index(lower, needle)
			if idx < 0 {
				break
			}
			end := idx + len(needle)
			for end < len(out) && !strings.ContainsRune(" \n\t\"'", rune(out[end])) {
				end++
			}
			out = out[:idx] + "redacted-url" + out[end:]
			lower = strings.ToLower(out)
		}
	}
	out = strings.ReplaceAll(out, osHomeHint(), "")
	return strings.TrimSpace(out)
}

func osHomeHint() string {
	return ""
}
