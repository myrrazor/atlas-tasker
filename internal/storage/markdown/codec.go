package markdown

import (
	"fmt"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"gopkg.in/yaml.v3"
)

type ticketFrontmatter struct {
	ID                   string                     `yaml:"id"`
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

func EncodeTicketMarkdown(ticket contracts.TicketSnapshot) (string, error) {
	ticket = contracts.NormalizeTicketSnapshot(ticket)
	fm := ticketFrontmatter{
		ID:                   ticket.ID,
		Project:              ticket.Project,
		Title:                ticket.Title,
		Type:                 ticket.Type,
		Status:               ticket.Status,
		Priority:             ticket.Priority,
		Parent:               ticket.Parent,
		Labels:               ticket.Labels,
		Assignee:             ticket.Assignee,
		Reviewer:             ticket.Reviewer,
		BlockedBy:            ticket.BlockedBy,
		Blocks:               ticket.Blocks,
		CreatedAt:            ticket.CreatedAt,
		UpdatedAt:            ticket.UpdatedAt,
		SchemaVersion:        ticket.SchemaVersion,
		Archived:             ticket.Archived,
		Policy:               ticket.Policy,
		ReviewState:          ticket.ReviewState,
		Lease:                ticket.Lease,
		Template:             ticket.Template,
		SkillHint:            ticket.SkillHint,
		Blueprint:            ticket.Blueprint,
		Progress:             ticket.Progress,
		RequiredCapabilities: ticket.RequiredCapabilities,
		DispatchMode:         ticket.DispatchMode,
		AllowParallelRuns:    ticket.AllowParallelRuns,
		Runbook:              ticket.Runbook,
		LatestRunID:          ticket.LatestRunID,
		LatestHandoffID:      ticket.LatestHandoffID,
		OpenGateIDs:          ticket.OpenGateIDs,
		LastDispatchAt:       ticket.LastDispatchAt,
		ChangeIDs:            ticket.ChangeIDs,
		ChangeReadyState:     ticket.ChangeReadyState,
		ChangeReadyReasons:   ticket.ChangeReadyReasons,
		PermissionProfiles:   ticket.PermissionProfiles,
		Protected:            ticket.Protected,
		Sensitive:            ticket.Sensitive,
	}
	rawFM, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal ticket frontmatter: %w", err)
	}

	acceptance := ""
	for _, criterion := range ticket.AcceptanceCriteria {
		trimmed := strings.TrimSpace(criterion)
		if trimmed == "" {
			continue
		}
		acceptance += "- " + trimmed + "\n"
	}
	acceptance = strings.TrimSuffix(acceptance, "\n")

	body := strings.TrimSpace(fmt.Sprintf(`# Summary

%s

## Description

%s

## Acceptance Criteria

%s

## Notes

%s
`,
		strings.TrimSpace(ticket.Summary),
		strings.TrimSpace(ticket.Description),
		acceptance,
		strings.TrimSpace(ticket.Notes),
	))

	doc := fmt.Sprintf("---\n%s---\n\n%s\n", string(rawFM), body)
	return doc, nil
}

func DecodeTicketMarkdown(doc string) (contracts.TicketSnapshot, error) {
	fmRaw, body, err := splitFrontmatter(doc)
	if err != nil {
		return contracts.TicketSnapshot{}, err
	}

	var fm ticketFrontmatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return contracts.TicketSnapshot{}, fmt.Errorf("unmarshal ticket frontmatter: %w", err)
	}

	summary, description, acceptance, notes := parseTicketBody(body)

	ticket := contracts.TicketSnapshot{
		ID:                   fm.ID,
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
	}

	return contracts.NormalizeTicketSnapshot(ticket), nil
}

func splitFrontmatter(doc string) (string, string, error) {
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

func parseTicketBody(body string) (summary string, description string, acceptance []string, notes string) {
	normalized := strings.ReplaceAll(strings.TrimSpace(body), "\r\n", "\n")
	sections := map[string]string{}
	current := ""
	var buf strings.Builder
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(buf.String())
			buf.Reset()
		}
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
			if current != "" {
				buf.WriteString(line)
				buf.WriteString("\n")
			}
		}
	}
	flush()

	summary = sections["summary"]
	description = sections["description"]
	notes = sections["notes"]
	acceptance = make([]string, 0)
	for _, line := range strings.Split(sections["acceptance"], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
		if trimmed != "" {
			acceptance = append(acceptance, trimmed)
		}
	}

	return summary, description, acceptance, notes
}
