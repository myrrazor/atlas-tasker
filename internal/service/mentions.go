package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

var (
	fencedCodePattern = regexp.MustCompile("(?s)```.*?```")
	inlineCodePattern = regexp.MustCompile("`[^`]*`")
	mentionPattern    = regexp.MustCompile(`(^|[\s\(\[\{>'",;])@([a-z0-9][a-z0-9._-]*)\b`)
)

type mentionExtraction struct {
	Mentions []contracts.Mention
	Warnings []string
}

func (s *ActionService) extractMentions(ctx context.Context, event contracts.Event, sourceKind string, sourceID string, ticketID string, texts ...string) (mentionExtraction, error) {
	available, err := s.Collaborators.ListCollaborators(ctx)
	if err != nil {
		return mentionExtraction{}, err
	}
	known := make(map[string]struct{}, len(available))
	for _, collaborator := range available {
		known[collaborator.CollaboratorID] = struct{}{}
	}
	workspaceID, err := ensureWorkspaceIdentity(s.Root)
	if err != nil {
		return mentionExtraction{}, err
	}
	candidates := collectMentionCandidates(texts...)
	result := mentionExtraction{Mentions: make([]contracts.Mention, 0, len(candidates))}
	for idx, collaboratorID := range candidates {
		if _, ok := known[collaboratorID]; !ok {
			result.Warnings = append(result.Warnings, collaboratorID)
			continue
		}
		result.Mentions = append(result.Mentions, normalizeMention(contracts.Mention{
			MentionUID:        contracts.MentionUID(event.EventUID, collaboratorID, idx+1),
			CollaboratorID:    collaboratorID,
			SourceKind:        strings.TrimSpace(sourceKind),
			SourceID:          strings.TrimSpace(sourceID),
			SourceEventUID:    event.EventUID,
			TicketID:          strings.TrimSpace(ticketID),
			OriginWorkspaceID: workspaceID,
			CreatedAt:         event.Timestamp,
		}))
	}
	result.Warnings = uniqueStrings(result.Warnings)
	return result, nil
}

func collectMentionCandidates(texts ...string) []string {
	items := make([]string, 0)
	for _, text := range texts {
		clean := sanitizeMentionText(text)
		matches := mentionPattern.FindAllStringSubmatch(clean, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			items = append(items, strings.TrimSpace(match[2]))
		}
	}
	return items
}

func sanitizeMentionText(raw string) string {
	raw = strings.ReplaceAll(raw, "\\@", " ")
	raw = fencedCodePattern.ReplaceAllString(raw, " ")
	raw = inlineCodePattern.ReplaceAllString(raw, " ")
	return raw
}

func mentionsForTicket(mentions []contracts.Mention, ticketID string) []contracts.Mention {
	if strings.TrimSpace(ticketID) == "" {
		return []contracts.Mention{}
	}
	items := make([]contracts.Mention, 0, len(mentions))
	for _, mention := range mentions {
		if mention.TicketID == ticketID {
			items = append(items, mention)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].MentionUID < items[j].MentionUID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

func mentionsForRun(mentions []contracts.Mention, runID string) []contracts.Mention {
	if strings.TrimSpace(runID) == "" {
		return []contracts.Mention{}
	}
	items := make([]contracts.Mention, 0, len(mentions))
	for _, mention := range mentions {
		if mention.SourceID == runID {
			items = append(items, mention)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].MentionUID < items[j].MentionUID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

func (s *ActionService) recordMentionEvents(ctx context.Context, project string, actor contracts.Actor, reason string, mentions []contracts.Mention) error {
	for _, mention := range mentions {
		event, err := s.newEvent(ctx, strings.TrimSpace(project), mention.CreatedAt, actor, reason, contracts.EventMentionRecorded, mention.TicketID, mention)
		if err != nil {
			return apperr.Wrap(apperr.CodeRepairNeeded, err, "allocate mention audit event")
		}
		if err := s.commitMutation(ctx, fmt.Sprintf("record mention %s", mention.MentionUID), "event_only", event, nil); err != nil {
			return apperr.Wrap(apperr.CodeRepairNeeded, err, "append mention audit event")
		}
	}
	return nil
}
