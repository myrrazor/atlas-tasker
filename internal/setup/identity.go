package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/integrations/adapter"
)

// InspectFile records path, existence, mode, size, owner, and SHA-256 without
// storing file contents. Symlinked targets are refused. A file owned by
// someone other than the current user is refused (transaction model §5).
func InspectFile(path string, currentUID int) (adapter.FileIdentity, error) {
	identity := adapter.FileIdentity{Path: path}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return identity, nil
	}
	if err != nil {
		return identity, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return identity, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("refusing symlinked path %s", path))
	}
	identity.Exists = true
	identity.Mode = uint32(info.Mode().Perm())
	identity.Size = info.Size()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		identity.Owner = &adapter.FileOwner{UID: stat.Uid, GID: stat.Gid}
		if currentUID >= 0 && int(stat.Uid) != currentUID && info.Mode().IsRegular() {
			return identity, apperr.New(apperr.CodePermissionDenied, fmt.Sprintf("file %s is not owned by the current user", path))
		}
	}
	if info.Mode().IsRegular() {
		sum, err := hashFile(path)
		if err != nil {
			return identity, err
		}
		identity.SHA256 = sum
	}
	return identity, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func identitiesEqual(a, b adapter.FileIdentity) bool {
	if a.Exists != b.Exists || a.Mode != b.Mode || a.Size != b.Size || a.SHA256 != b.SHA256 {
		return false
	}
	if a.Owner == nil && b.Owner == nil {
		return true
	}
	if a.Owner == nil || b.Owner == nil {
		return false
	}
	return a.Owner.UID == b.Owner.UID && a.Owner.GID == b.Owner.GID
}

func fileDevIno(path string) (uint64, uint64, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Dev), uint64(stat.Ino), true
}
