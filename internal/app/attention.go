package app

import (
	"context"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/service"
)

func (a *App) Attention(ctx context.Context, opts AttentionOptions) (AttentionReport, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	report := AttentionReport{Kind: "attention"}
	workspaces, err := a.ListWorkspaces(ctx, ListOptions{IncludeHidden: a.snapshotSettings().Home.ShowHidden})
	if err != nil {
		return AttentionReport{}, err
	}
	for _, rec := range workspaces {
		if rec.Health != HealthAvailable {
			report.Missing = append(report.Missing, rec)
			report.Items = append(report.Items, AttentionItem{
				WorkspaceID: rec.WorkspaceID,
				Path:        rec.Path,
				DisplayName: rec.DisplayName,
				Category:    "workspace",
				Reason:      rec.HealthDetail,
				Health:      rec.Health,
			})
			continue
		}
		ws, err := a.hub.Bind(ctx, rec.WorkspaceID)
		if err != nil {
			report.Missing = append(report.Missing, rec)
			continue
		}
		actor := opts.Actor
		if actor == "" {
			actor, _ = ws.Queries.ResolveActor(ctx, "")
		}
		queue, err := ws.Queries.Queue(ctx, actor)
		if err != nil {
			continue
		}
		order := []service.QueueCategory{
			service.QueueBlockedForMe,
			service.QueueStaleClaims,
			service.QueueNeedsReview,
			service.QueueAwaitingOwner,
			service.QueueReadyForMe,
			service.QueueUnblockedForMe,
		}
		for _, cat := range order {
			for _, entry := range queue.Categories[cat] {
				if len(report.Items) >= limit {
					return report, nil
				}
				report.Items = append(report.Items, AttentionItem{
					WorkspaceID: rec.WorkspaceID,
					Path:        rec.Path,
					DisplayName: rec.DisplayName,
					TicketID:    entry.Ticket.ID,
					Project:     entry.Ticket.Project,
					Title:       entry.Ticket.Title,
					Category:    string(cat),
					Reason:      firstNonEmpty(entry.Reason, string(cat)),
					Health:      rec.Health,
				})
			}
		}
	}
	return report, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (a *App) Search(ctx context.Context, opts SearchOptions) (SearchReport, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 40
	}
	report := SearchReport{Kind: "search", Query: opts.Query}
	if strings.TrimSpace(opts.Query) == "" {
		return report, nil
	}
	query, err := contracts.ParseSearchQuery(opts.Query)
	if err != nil {
		return SearchReport{}, err
	}
	workspaces, err := a.ListWorkspaces(ctx, ListOptions{})
	if err != nil {
		return SearchReport{}, err
	}
	for _, rec := range workspaces {
		if rec.Health != HealthAvailable {
			continue
		}
		ws, err := a.hub.Bind(ctx, rec.WorkspaceID)
		if err != nil {
			continue
		}
		tickets, err := ws.Queries.Search(ctx, query)
		if err != nil {
			continue
		}
		for _, ticket := range tickets {
			if len(report.Hits) >= limit {
				return report, nil
			}
			report.Hits = append(report.Hits, SearchHit{
				WorkspaceID: rec.WorkspaceID,
				TicketID:    ticket.ID,
				Project:     ticket.Project,
				Title:       ticket.Title,
				Status:      string(ticket.Status),
			})
		}
	}
	return report, nil
}
