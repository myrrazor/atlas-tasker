package host

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/integrations"
)

func shortHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:12]
}

func installerPreview(root string, target integrations.Target) []integrations.PlannedInstallFile {
	files, err := (integrations.Installer{Root: root}).Preview(target)
	if err != nil {
		return nil
	}
	return files
}

func instructionMarkers(target integrations.Target) (string, string, error) {
	return integrations.InstructionMarkers(target)
}

func stripBlock(body, begin, end string) (string, bool) {
	start := strings.Index(body, begin)
	stop := strings.Index(body, end)
	if start < 0 || stop < 0 || stop < start {
		return body, false
	}
	stop += len(end)
	if stop < len(body) && body[stop] == '\n' {
		stop++
	}
	updated := body[:start] + body[stop:]
	return updated, updated != body
}
