package service

import (
	"context"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func resolveEffectivePolicy(ctx context.Context, root string, projects contracts.ProjectStore, tickets contracts.TicketStore, ticket contracts.TicketSnapshot) (EffectivePolicyView, error) {
	cfg, err := config.Load(root)
	if err != nil {
		return EffectivePolicyView{}, err
	}
	project, err := projects.GetProject(ctx, ticket.Project)
	if err != nil {
		return EffectivePolicyView{}, err
	}

	view := EffectivePolicyView{
		CompletionMode: cfg.Workflow.CompletionMode,
		LeaseTTL:       contracts.DefaultLeaseTTL,
		Sources:        []PolicySource{PolicySourceLegacy},
	}
	if project.Defaults.CompletionMode != "" {
		view.CompletionMode = project.Defaults.CompletionMode
		view.Sources = append(view.Sources, PolicySourceProject)
	}
	if project.Defaults.LeaseTTLMinutes > 0 {
		view.LeaseTTL = time.Duration(project.Defaults.LeaseTTLMinutes) * time.Minute
	}
	if len(project.Defaults.AllowedWorkers) > 0 {
		view.AllowedWorkers = append([]contracts.Actor{}, project.Defaults.AllowedWorkers...)
	}
	if project.Defaults.RequiredReviewer != "" {
		view.RequiredReviewer = project.Defaults.RequiredReviewer
	}
	if ticket.Parent != "" {
		parent, err := tickets.GetTicket(ctx, ticket.Parent)
		if err == nil && parent.Type == contracts.TicketTypeEpic && parent.Policy.HasOverrides() {
			applyPolicy(&view, parent.Policy)
			view.Sources = append(view.Sources, PolicySourceEpic)
		}
	}
	if ticket.Policy.HasOverrides() {
		applyPolicy(&view, ticket.Policy)
		view.Sources = append(view.Sources, PolicySourceTicket)
	}
	return view, nil
}
