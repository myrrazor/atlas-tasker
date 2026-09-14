package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/myrrazor/atlas-tasker/internal/apperr"
)

func settingsPath(stateDir string) string {
	return filepath.Join(stateDir, "settings.json")
}

func defaultSettings() MachineSettings {
	return MachineSettings{
		Format:     settingsFormat,
		InstanceID: uuid.NewString(),
		Service: ServiceSettings{
			Bind:      DefaultHomeBind,
			Port:      DefaultHomePort,
			Enabled:   true,
			AutoStart: true,
		},
		Browser:          BrowserSettings{OpenHome: true},
		AutoRegister:     true,
		Agents:           AgentSettings{AutoInstall: true},
		DefaultProject:   true,
		LocalCheckpoints: true,
		Discovery:        DiscoverySettings{Enabled: false, Roots: []string{}, MaxDepth: 4},
		Home:             HomeSettings{ShowHidden: false},
		GitMode:          GitModeShared,
	}
}

func (a *App) loadSettings() (MachineSettings, error) {
	defaults := defaultSettings()
	raw, err := os.ReadFile(settingsPath(a.stateDir))
	if os.IsNotExist(err) {
		if err := atomicJSON(settingsPath(a.stateDir), defaults); err != nil {
			return MachineSettings{}, err
		}
		return defaults, nil
	}
	if err != nil {
		return MachineSettings{}, fmt.Errorf("read machine settings: %w", err)
	}
	var got MachineSettings
	if err := json.Unmarshal(raw, &got); err != nil {
		return MachineSettings{}, fmt.Errorf("decode machine settings: %w", err)
	}
	if got.Format != "" && got.Format != settingsFormat {
		return MachineSettings{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("unsupported machine settings format %q", got.Format))
	}
	merged := defaults
	_ = json.Unmarshal(raw, &merged)
	if merged.InstanceID == "" {
		merged.InstanceID = defaults.InstanceID
	}
	if merged.Service.Port == 0 {
		merged.Service.Port = DefaultHomePort
	}
	if merged.Service.Bind == "" {
		merged.Service.Bind = DefaultHomeBind
	}
	if merged.GitMode == "" {
		merged.GitMode = GitModeShared
	}
	if merged.Discovery.MaxDepth == 0 {
		merged.Discovery.MaxDepth = 4
	}
	merged.Format = settingsFormat
	return merged, nil
}

func cloneSettings(in MachineSettings) MachineSettings {
	out := in
	if in.Discovery.Roots != nil {
		out.Discovery.Roots = append([]string{}, in.Discovery.Roots...)
	}
	return out
}

func (a *App) snapshotSettings() MachineSettings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneSettings(a.settings)
}

func (a *App) Settings() MachineSettings {
	return a.snapshotSettings()
}

func (a *App) UpdateSettings(ctx context.Context, patch MachineSettingsPatch) (MachineSettings, error) {
	_ = ctx
	a.mu.Lock()
	defer a.mu.Unlock()
	next := cloneSettings(a.settings)
	if patch.Service != nil {
		next.Service = *patch.Service
		if next.Service.Port == 0 {
			next.Service.Port = DefaultHomePort
		}
		if next.Service.Bind == "" {
			next.Service.Bind = DefaultHomeBind
		}
	}
	if patch.Browser != nil {
		next.Browser = *patch.Browser
	}
	if patch.AutoRegister != nil {
		next.AutoRegister = *patch.AutoRegister
	}
	if patch.Agents != nil {
		next.Agents = *patch.Agents
	}
	if patch.DefaultProject != nil {
		next.DefaultProject = *patch.DefaultProject
	}
	if patch.LocalCheckpoints != nil {
		next.LocalCheckpoints = *patch.LocalCheckpoints
	}
	if patch.Discovery != nil {
		next.Discovery = *patch.Discovery
	}
	if patch.Home != nil {
		next.Home = *patch.Home
	}
	if patch.GitMode != nil {
		if !patch.GitMode.IsValid() {
			return MachineSettings{}, apperr.New(apperr.CodeInvalidInput, "invalid git mode")
		}
		next.GitMode = patch.GitMode.Normalized()
	}
	next.Format = settingsFormat
	if err := atomicJSON(settingsPath(a.stateDir), next); err != nil {
		return MachineSettings{}, err
	}
	a.settings = cloneSettings(next)
	return cloneSettings(next), nil
}
