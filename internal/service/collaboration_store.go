package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"gopkg.in/yaml.v3"
)

type CollaboratorStore struct {
	Root string
}

type MembershipStore struct {
	Root string
}

type MentionStore struct {
	Root string
}

type collaboratorFrontmatter struct {
	contracts.CollaboratorProfile `yaml:",inline"`
}

type membershipFrontmatter struct {
	contracts.MembershipBinding `yaml:",inline"`
}

type mentionFrontmatter struct {
	contracts.Mention `yaml:",inline"`
}

func (s CollaboratorStore) SaveCollaborator(_ context.Context, collaborator contracts.CollaboratorProfile) error {
	collaborator = normalizeCollaborator(collaborator)
	if err := collaborator.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.CollaboratorsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create collaborators dir: %w", err)
	}
	raw, err := yaml.Marshal(collaboratorFrontmatter{CollaboratorProfile: collaborator})
	if err != nil {
		return fmt.Errorf("marshal collaborator %s: %w", collaborator.CollaboratorID, err)
	}
	body := strings.TrimSpace(collaborator.DisplayName)
	if body == "" {
		body = collaborator.CollaboratorID
	}
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.CollaboratorFile(s.Root, collaborator.CollaboratorID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write collaborator %s: %w", collaborator.CollaboratorID, err)
	}
	return nil
}

func (s CollaboratorStore) LoadCollaborator(_ context.Context, collaboratorID string) (contracts.CollaboratorProfile, error) {
	collaboratorID = strings.TrimSpace(collaboratorID)
	if !contracts.IsValidCollaboratorID(collaboratorID) {
		return contracts.CollaboratorProfile{}, fmt.Errorf("%s", contracts.CollaboratorIDValidationMessage())
	}
	raw, err := os.ReadFile(storage.CollaboratorFile(s.Root, collaboratorID))
	if err != nil {
		return contracts.CollaboratorProfile{}, fmt.Errorf("read collaborator %s: %w", collaboratorID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.CollaboratorProfile{}, err
	}
	var fm collaboratorFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.CollaboratorProfile{}, fmt.Errorf("parse collaborator %s: %w", collaboratorID, err)
	}
	collaborator := normalizeCollaborator(fm.CollaboratorProfile)
	return collaborator, collaborator.Validate()
}

func (s CollaboratorStore) ListCollaborators(_ context.Context) ([]contracts.CollaboratorProfile, error) {
	entries, err := os.ReadDir(storage.CollaboratorsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.CollaboratorProfile{}, nil
		}
		return nil, fmt.Errorf("read collaborators dir: %w", err)
	}
	items := make([]contracts.CollaboratorProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		collaboratorID := strings.TrimSuffix(entry.Name(), ".md")
		collaborator, err := s.LoadCollaborator(context.Background(), collaboratorID)
		if err != nil {
			return nil, err
		}
		items = append(items, collaborator)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CollaboratorID < items[j].CollaboratorID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s MembershipStore) SaveMembership(_ context.Context, membership contracts.MembershipBinding) error {
	membership = normalizeMembership(membership)
	if err := membership.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.MembershipsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create memberships dir: %w", err)
	}
	raw, err := yaml.Marshal(membershipFrontmatter{MembershipBinding: membership})
	if err != nil {
		return fmt.Errorf("marshal membership %s: %w", membership.MembershipUID, err)
	}
	body := fmt.Sprintf("%s -> %s:%s (%s)", membership.CollaboratorID, membership.ScopeKind, membership.ScopeID, membership.Role)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.MembershipFile(s.Root, membership.MembershipUID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write membership %s: %w", membership.MembershipUID, err)
	}
	return nil
}

func (s MembershipStore) LoadMembership(_ context.Context, membershipUID string) (contracts.MembershipBinding, error) {
	raw, err := os.ReadFile(storage.MembershipFile(s.Root, strings.TrimSpace(membershipUID)))
	if err != nil {
		return contracts.MembershipBinding{}, fmt.Errorf("read membership %s: %w", membershipUID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.MembershipBinding{}, err
	}
	var fm membershipFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.MembershipBinding{}, fmt.Errorf("parse membership %s: %w", membershipUID, err)
	}
	membership := normalizeMembership(fm.MembershipBinding)
	return membership, membership.Validate()
}

func (s MembershipStore) ListMemberships(_ context.Context, collaboratorID string) ([]contracts.MembershipBinding, error) {
	entries, err := os.ReadDir(storage.MembershipsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.MembershipBinding{}, nil
		}
		return nil, fmt.Errorf("read memberships dir: %w", err)
	}
	filterID := strings.TrimSpace(collaboratorID)
	items := make([]contracts.MembershipBinding, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		membershipUID := strings.TrimSuffix(entry.Name(), ".md")
		membership, err := s.LoadMembership(context.Background(), membershipUID)
		if err != nil {
			return nil, err
		}
		if filterID != "" && membership.CollaboratorID != filterID {
			continue
		}
		items = append(items, membership)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].MembershipUID < items[j].MembershipUID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s MentionStore) SaveMention(_ context.Context, mention contracts.Mention) error {
	mention = normalizeMention(mention)
	if err := mention.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.MentionsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create mentions dir: %w", err)
	}
	raw, err := yaml.Marshal(mentionFrontmatter{Mention: mention})
	if err != nil {
		return fmt.Errorf("marshal mention %s: %w", mention.MentionUID, err)
	}
	body := fmt.Sprintf("%s mentioned in %s %s", mention.CollaboratorID, mention.SourceKind, mention.SourceID)
	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(raw), body)
	if err := os.WriteFile(storage.MentionFile(s.Root, mention.MentionUID), []byte(doc), 0o644); err != nil {
		return fmt.Errorf("write mention %s: %w", mention.MentionUID, err)
	}
	return nil
}

func (s MentionStore) LoadMention(_ context.Context, mentionUID string) (contracts.Mention, error) {
	raw, err := os.ReadFile(storage.MentionFile(s.Root, strings.TrimSpace(mentionUID)))
	if err != nil {
		return contracts.Mention{}, fmt.Errorf("read mention %s: %w", mentionUID, err)
	}
	fmRaw, _, err := splitDocument(string(raw))
	if err != nil {
		return contracts.Mention{}, err
	}
	var fm mentionFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.Mention{}, fmt.Errorf("parse mention %s: %w", mentionUID, err)
	}
	mention := normalizeMention(fm.Mention)
	return mention, mention.Validate()
}

func (s MentionStore) ListMentions(_ context.Context, collaboratorID string) ([]contracts.Mention, error) {
	entries, err := os.ReadDir(storage.MentionsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.Mention{}, nil
		}
		return nil, fmt.Errorf("read mentions dir: %w", err)
	}
	filterID := strings.TrimSpace(collaboratorID)
	items := make([]contracts.Mention, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		mentionUID := strings.TrimSuffix(entry.Name(), ".md")
		mention, err := s.LoadMention(context.Background(), mentionUID)
		if err != nil {
			return nil, err
		}
		if filterID != "" && mention.CollaboratorID != filterID {
			continue
		}
		items = append(items, mention)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].MentionUID < items[j].MentionUID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func normalizeCollaborator(collaborator contracts.CollaboratorProfile) contracts.CollaboratorProfile {
	collaborator.CollaboratorID = strings.TrimSpace(collaborator.CollaboratorID)
	collaborator.DisplayName = strings.TrimSpace(collaborator.DisplayName)
	if collaborator.Status == "" {
		collaborator.Status = contracts.CollaboratorStatusActive
	}
	if collaborator.TrustState == "" {
		collaborator.TrustState = contracts.CollaboratorTrustStateUntrusted
	}
	if collaborator.CreatedAt.IsZero() {
		collaborator.CreatedAt = timeNowUTC()
	}
	if collaborator.UpdatedAt.IsZero() {
		collaborator.UpdatedAt = collaborator.CreatedAt
	}
	if collaborator.SchemaVersion == 0 {
		collaborator.SchemaVersion = contracts.CurrentSchemaVersion
	}
	if collaborator.ProviderHandles == nil {
		collaborator.ProviderHandles = map[string]string{}
	}
	collaborator.AtlasActors = uniqueActors(collaborator.AtlasActors)
	return collaborator
}

func normalizeMembership(membership contracts.MembershipBinding) contracts.MembershipBinding {
	membership.CollaboratorID = strings.TrimSpace(membership.CollaboratorID)
	membership.ScopeID = strings.TrimSpace(membership.ScopeID)
	if membership.MembershipUID == "" && membership.CollaboratorID != "" && membership.ScopeKind != "" && membership.ScopeID != "" && membership.Role != "" {
		membership.MembershipUID = contracts.MembershipUID(membership.CollaboratorID, membership.ScopeKind, membership.ScopeID, membership.Role)
	}
	if membership.Status == "" {
		membership.Status = contracts.MembershipStatusActive
	}
	if membership.CreatedAt.IsZero() {
		membership.CreatedAt = timeNowUTC()
	}
	if membership.UpdatedAt.IsZero() {
		membership.UpdatedAt = membership.CreatedAt
	}
	membership.DefaultPermissionProfiles = uniqueStrings(membership.DefaultPermissionProfiles)
	return membership
}

func normalizeMention(mention contracts.Mention) contracts.Mention {
	mention.MentionUID = strings.TrimSpace(mention.MentionUID)
	mention.CollaboratorID = strings.TrimSpace(mention.CollaboratorID)
	mention.SourceKind = strings.TrimSpace(mention.SourceKind)
	mention.SourceID = strings.TrimSpace(mention.SourceID)
	mention.SourceEventUID = strings.TrimSpace(mention.SourceEventUID)
	mention.TicketID = strings.TrimSpace(mention.TicketID)
	mention.OriginWorkspaceID = strings.TrimSpace(mention.OriginWorkspaceID)
	if mention.CreatedAt.IsZero() {
		mention.CreatedAt = timeNowUTC()
	}
	return mention
}

func uniqueActors(values []contracts.Actor) []contracts.Actor {
	seen := make(map[contracts.Actor]struct{}, len(values))
	items := make([]contracts.Actor, 0, len(values))
	for _, value := range values {
		value = contracts.Actor(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	sort.Strings(items)
	return items
}
