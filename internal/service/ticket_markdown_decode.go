package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"gopkg.in/yaml.v3"
)

type ticketConflictFrontmatter struct {
	ID                   string                     `yaml:"id"`
	TicketUID            string                     `yaml:"ticket_uid,omitempty"`
	Project              string                     `yaml:"project"`
	Title                string                     `yaml:"title"`
	Type                 contracts.TicketType       `yaml:"type"`
	Status               contracts.Status           `yaml:"status"`
	Priority             contracts.Priority         `yaml:"priority"`
	Parent               string                     `yaml:"parent,omitempty"`
	Labels               []string                   `yaml:"labels"`
	Assignee             contracts.Actor            `yaml:"assignee,omitempty"`
	Reviewer             contracts.Actor            `yaml:"reviewer,omitempty"`
	BlockedBy            []string                   `yaml:"blocked_by"`
	Blocks               []string                   `yaml:"blocks"`
	CreatedAt            time.Time                  `yaml:"created_at"`
	UpdatedAt            time.Time                  `yaml:"updated_at"`
	SchemaVersion        int                        `yaml:"schema_version"`
	Archived             bool                       `yaml:"archived,omitempty"`
	Policy               contracts.TicketPolicy     `yaml:"policy,omitempty"`
	ReviewState          contracts.ReviewState      `yaml:"review_state,omitempty"`
	Lease                contracts.LeaseState       `yaml:"lease,omitempty"`
	Template             string                     `yaml:"template,omitempty"`
	SkillHint            string                     `yaml:"skill_hint,omitempty"`
	Blueprint            string                     `yaml:"blueprint,omitempty"`
	Progress             contracts.ProgressSummary  `yaml:"progress,omitempty"`
	RequiredCapabilities []string                   `yaml:"required_capabilities,omitempty"`
	DispatchMode         contracts.DispatchMode     `yaml:"dispatch_mode,omitempty"`
	AllowParallelRuns    bool                       `yaml:"allow_parallel_runs,omitempty"`
	Runbook              string                     `yaml:"runbook,omitempty"`
	LatestRunID          string                     `yaml:"latest_run_id,omitempty"`
	LatestHandoffID      string                     `yaml:"latest_handoff_id,omitempty"`
	OpenGateIDs          []string                   `yaml:"open_gate_ids,omitempty"`
	LastDispatchAt       time.Time                  `yaml:"last_dispatch_at,omitempty"`
	ChangeIDs            []string                   `yaml:"change_ids,omitempty"`
	ChangeReadyState     contracts.ChangeReadyState `yaml:"change_ready_state,omitempty"`
	ChangeReadyReasons   []string                   `yaml:"change_ready_reasons,omitempty"`
	PermissionProfiles   []string                   `yaml:"permission_profiles,omitempty"`
	Protected            bool                       `yaml:"protected,omitempty"`
	Sensitive            bool                       `yaml:"sensitive,omitempty"`
}

func decodeTicketConflictSnapshot(doc string) (contracts.TicketSnapshot, error) {
	fmRaw, body, err := splitTicketConflictFrontmatter(doc)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}

	var fm ticketConflictFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.TicketSnapshot{}, fmt.Errorf("unmarshal ticket frontmatter: %w", err)
	}

	summary, description, acceptance, notes := parseTicketConflictBody(body)
	return contracts.NormalizeTicketSnapshot(contracts.TicketSnapshot{
		ID:                   fm.ID,
		TicketUID:            strings.TrimSpace(fm.TicketUID),
		Project:              fm.Project,
		Title:                fm.Title,
		Type:                 fm.Type,
		Status:               fm.Status,
		Priority:             fm.Priority,
		Parent:               fm.Parent,
		Labels:               fm.Labels,
		Assignee:             fm.Assignee,
		Reviewer:             fm.Reviewer,
		BlockedBy:            fm.BlockedBy,
		Blocks:               fm.Blocks,
		CreatedAt:            fm.CreatedAt,
		UpdatedAt:            fm.UpdatedAt,
		SchemaVersion:        fm.SchemaVersion,
		Archived:             fm.Archived,
		Policy:               fm.Policy,
		ReviewState:          fm.ReviewState,
		Lease:                fm.Lease,
		Template:             strings.TrimSpace(fm.Template),
		SkillHint:            strings.TrimSpace(fm.SkillHint),
		Blueprint:            strings.TrimSpace(fm.Blueprint),
		Progress:             fm.Progress,
		RequiredCapabilities: fm.RequiredCapabilities,
		DispatchMode:         fm.DispatchMode,
		AllowParallelRuns:    fm.AllowParallelRuns,
		Runbook:              strings.TrimSpace(fm.Runbook),
		LatestRunID:          strings.TrimSpace(fm.LatestRunID),
		LatestHandoffID:      strings.TrimSpace(fm.LatestHandoffID),
		OpenGateIDs:          fm.OpenGateIDs,
		LastDispatchAt:       fm.LastDispatchAt,
		ChangeIDs:            fm.ChangeIDs,
		ChangeReadyState:     fm.ChangeReadyState,
		ChangeReadyReasons:   fm.ChangeReadyReasons,
		PermissionProfiles:   fm.PermissionProfiles,
		Protected:            fm.Protected,
		Sensitive:            fm.Sensitive,
		Summary:              summary,
		Description:          description,
		AcceptanceCriteria:   acceptance,
		Notes:                notes,
	}), nil
}

func splitTicketConflictFrontmatter(doc string) (string, string, error) {
	normalized := strings.ReplaceAll(doc, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", "", fmt.Errorf("missing frontmatter start")
	}
	parts := strings.SplitN(strings.TrimPrefix(normalized, "---\n"), "\n---\n", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("missing frontmatter end")
	}
	return parts[0], parts[1], nil
}

func parseTicketConflictBody(body string) (summary string, description string, acceptance []string, notes string) {
	normalized := strings.ReplaceAll(strings.TrimSpace(body), "\r\n", "\n")
	sections := map[string]string{}
	current := ""
	var buf strings.Builder
	flush := func() {
		if current == "" {
			return
		}
		sections[current] = strings.TrimSpace(buf.String())
		buf.Reset()
	}

	for _, line := range strings.Split(normalized, "\n") {
		switch strings.TrimSpace(line) {
		case "# Summary":
			flush()
			current = "summary"
		case "## Description":
			flush()
			current = "description"
		case "## Acceptance Criteria":
			flush()
			current = "acceptance"
		case "## Notes":
			flush()
			current = "notes"
		default:
			if current == "" {
				continue
			}
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	flush()

	summary = sections["summary"]
	description = sections["description"]
	notes = sections["notes"]
	for _, line := range strings.Split(sections["acceptance"], "\n") {
		item := strings.TrimSpace(line)
		if strings.HasPrefix(item, "- ") {
			item = strings.TrimSpace(strings.TrimPrefix(item, "- "))
		}
		if item != "" {
			acceptance = append(acceptance, item)
		}
	}
	return summary, description, acceptance, notes
}
