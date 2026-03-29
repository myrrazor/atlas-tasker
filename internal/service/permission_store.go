package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

type PermissionProfileStore struct {
	Root string
}

func (s PermissionProfileStore) SavePermissionProfile(_ context.Context, profile contracts.PermissionProfile) error {
	profile.ProfileID = sanitizePermissionProfileID(profile.ProfileID)
	if profile.SchemaVersion == 0 {
		profile.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.PermissionProfilesDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create permission profiles dir: %w", err)
	}
	raw, err := toml.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode permission profile %s: %w", profile.ProfileID, err)
	}
	if err := os.WriteFile(s.path(profile.ProfileID), raw, 0o644); err != nil {
		return fmt.Errorf("write permission profile %s: %w", profile.ProfileID, err)
	}
	return nil
}

func (s PermissionProfileStore) LoadPermissionProfile(_ context.Context, profileID string) (contracts.PermissionProfile, error) {
	raw, err := os.ReadFile(s.path(profileID))
	if err != nil {
		return contracts.PermissionProfile{}, fmt.Errorf("read permission profile %s: %w", profileID, err)
	}
	var profile contracts.PermissionProfile
	if err := toml.Unmarshal(raw, &profile); err != nil {
		return contracts.PermissionProfile{}, fmt.Errorf("parse permission profile %s: %w", profileID, err)
	}
	if strings.TrimSpace(profile.ProfileID) == "" {
		profile.ProfileID = sanitizePermissionProfileID(profileID)
	}
	if profile.SchemaVersion == 0 {
		profile.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if err := profile.Validate(); err != nil {
		return contracts.PermissionProfile{}, err
	}
	return profile, nil
}

func (s PermissionProfileStore) ListPermissionProfiles(_ context.Context) ([]contracts.PermissionProfile, error) {
	entries, err := os.ReadDir(storage.PermissionProfilesDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.PermissionProfile{}, nil
		}
		return nil, fmt.Errorf("read permission profiles dir: %w", err)
	}
	items := make([]contracts.PermissionProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		profile, err := s.LoadPermissionProfile(context.Background(), strings.TrimSuffix(entry.Name(), ".toml"))
		if err != nil {
			return nil, err
		}
		items = append(items, profile)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority != items[j].Priority {
			return items[i].Priority > items[j].Priority
		}
		return items[i].ProfileID < items[j].ProfileID
	})
	return items, nil
}

func (s PermissionProfileStore) path(profileID string) string {
	return filepath.Join(storage.PermissionProfilesDir(s.Root), sanitizePermissionProfileID(profileID)+".toml")
}

func sanitizePermissionProfileID(profileID string) string {
	profileID = strings.TrimSpace(strings.ToLower(profileID))
	profileID = strings.ReplaceAll(profileID, " ", "-")
	profileID = strings.ReplaceAll(profileID, "/", "-")
	profileID = strings.ReplaceAll(profileID, "_", "-")
	if profileID == "" {
		return "profile"
	}
	return profileID
}
