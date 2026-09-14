package app

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/uninstall"
)

func (a *App) recordHomeServiceUninstallAction(spec ServiceUnit) error {
	goos := runtime.GOOS
	action, err := uninstall.NewHomeServiceAction(a.home, goos, spec.Executable, "")
	if err != nil {
		return err
	}
	if spec.Label != "" && goos == "darwin" {
		action.UnitName = spec.Label
		action.Path = filepath.Join(a.home, "Library", "LaunchAgents", spec.Label+".plist")
	}
	return a.publishUninstallActions([]uninstall.ManifestAction{action})
}

func (a *App) recordUninstallClientActions(command string, args []string, clients []AgentClientReport) error {
	var actions []uninstall.ManifestAction
	for _, client := range clients {
		path := strings.TrimSpace(client.ConfigPath)
		if path == "" {
			continue
		}
		id := "mcp-" + string(client.Target)
		clientArgs := client.Args
		if len(clientArgs) == 0 {
			clientArgs = args
		}
		switch {
		case strings.HasSuffix(path, ".toml"):
			actions = append(actions, uninstall.NewManagedTOMLAction(id, path, GlobalMCPServerName, command, clientArgs))
		default:
			actions = append(actions, uninstall.NewManagedJSONAction(id, path, GlobalMCPServerName, command, clientArgs))
		}
	}
	return a.publishUninstallActions(actions)
}

// publishUninstallActions writes a receipt-bound uninstall manifest. No receipt
// means this is a development binary we do not own — leave the disk alone.
func (a *App) publishUninstallActions(extra []uninstall.ManifestAction) error {
	if len(extra) == 0 {
		return nil
	}
	receipt, err := uninstall.LoadReceipt(a.stateDir)
	if err != nil {
		return nil
	}
	if err := uninstall.ValidateReceiptBinary(receipt); err != nil {
		return nil
	}
	existing, err := uninstall.LoadManifest(a.stateDir)
	if err != nil {
		existing = uninstall.Manifest{}
	}
	if existing.Format != "" && existing.ReceiptDigest != receipt.Digest {
		rebound, rebindErr := uninstall.RebindManifest(existing, receipt, a.now())
		if rebindErr != nil {
			existing = uninstall.Manifest{}
		} else {
			existing = rebound
		}
	}
	merged := mergeUninstallManifestActions(existing.Actions, extra)
	for i := range merged {
		if merged[i].BinaryPath == "" {
			merged[i].BinaryPath = receipt.BinaryPath
			merged[i].BinarySHA256 = receipt.BinarySHA256
		}
	}
	manifest, err := uninstall.NewManifest(receipt, merged, a.now())
	if err != nil {
		return err
	}
	return uninstall.WriteManifest(a.stateDir, manifest)
}

func mergeUninstallManifestActions(groups ...[]uninstall.ManifestAction) []uninstall.ManifestAction {
	seen := map[string]struct{}{}
	var out []uninstall.ManifestAction
	for _, group := range groups {
		for _, action := range group {
			key := action.Kind + "|" + action.Path + "|" + action.UnitName + "|" + action.EntryKey + "|" + action.Marker
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, action)
		}
	}
	return out
}
