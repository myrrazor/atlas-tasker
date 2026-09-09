package service

import (
	"context"
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/apperr"
	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type pausedProjectListing struct {
	contracts.ProjectStore
	listed chan struct{}
	resume chan struct{}
}

func (s pausedProjectListing) ListProjects(ctx context.Context) ([]contracts.Project, error) {
	projects, err := s.ProjectStore.ListProjects(ctx)
	close(s.listed)
	<-s.resume
	return projects, err
}

func TestTeamPresetMigrationDoesNotLoseConcurrentPolicyWrite(t *testing.T) {
	_, ctx, actions := setupTeamPresetTest(t)
	if err := actions.CreateProject(ctx, contracts.Project{
		Key: "APP", Name: "App", CreatedAt: actions.now(),
		Defaults: contracts.ProjectDefaults{CompletionMode: contracts.CompletionModeOpen},
	}); err != nil {
		t.Fatal(err)
	}
	other := *actions
	listing := pausedProjectListing{ProjectStore: actions.Projects, listed: make(chan struct{}), resume: make(chan struct{})}
	actions.Projects = listing
	applied := make(chan error, 1)
	go func() {
		_, err := actions.ApplyTeamPreset(ctx, "pair", "", false, "human:owner", "require review")
		applied <- err
	}()
	select {
	case <-listing.listed:
	case err := <-applied:
		t.Fatalf("preset stopped before project migration: %v", err)
	}
	stronger := contracts.ProjectDefaults{
		CompletionMode: contracts.CompletionModeOwnerGate, RequiredReviewer: "human:specialist",
		AllowedWorkers: []contracts.Actor{"agent:designated"},
	}
	// Bound the competing command's wait so the test can release migration.
	// A successful competing write must survive; a busy writer retries after it.
	contender, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, writeErr := other.SetProjectPolicy(contender, "APP", stronger, "human:owner", "stronger policy")
	cancel()
	close(listing.resume)
	if err := <-applied; err != nil {
		t.Fatal(err)
	}
	if apperr.CodeOf(writeErr) == apperr.CodeBusy {
		if _, err := other.SetProjectPolicy(ctx, "APP", stronger, "human:owner", "retry stronger policy"); err != nil {
			t.Fatal(err)
		}
	} else if writeErr != nil {
		t.Fatal(writeErr)
	}
	project, err := other.Projects.GetProject(ctx, "APP")
	if err != nil || project.Defaults.CompletionMode != stronger.CompletionMode || project.Defaults.RequiredReviewer != stronger.RequiredReviewer || len(project.Defaults.AllowedWorkers) != 1 || project.Defaults.AllowedWorkers[0] != stronger.AllowedWorkers[0] {
		t.Fatalf("preset lost a successful concurrent policy change: %#v %v", project, err)
	}
}

func TestClaimRejectsAnotherAssigneesWork(t *testing.T) {
	for _, actor := range []contracts.Actor{"agent:builder-2", "human:other", "human:owner"} {
		t.Run(string(actor), func(t *testing.T) {
			_, ctx, actions, _, tickets, now := setupScheduleTest(t, nil)
			ticket := scheduleTestTicket("APP-1", contracts.StatusReady, now)
			ticket.Assignee = "agent:builder-1"
			if err := tickets.CreateTicket(ctx, ticket); err != nil {
				t.Fatal(err)
			}
			_, err := actions.ClaimTicket(ctx, ticket.ID, actor, "take work")
			if apperr.CodeOf(err) != apperr.CodeConflict {
				t.Fatalf("another assignee's work must conflict, got %v", err)
			}
			after, err := tickets.GetTicket(ctx, ticket.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Lease.Actor != "" || after.Assignee != ticket.Assignee {
				t.Fatalf("rejected claim mutated ticket: %#v", after)
			}
			if _, err := actions.ClaimTicket(ctx, ticket.ID, ticket.Assignee, "assigned work"); err != nil {
				t.Fatalf("assignee cannot claim: %v", err)
			}
		})
	}
}

func TestTicketApprovalEnforcesPermissionProfile(t *testing.T) {
	root, ctx, actions, _, tickets, now := setupScheduleTest(t, nil)
	if _, err := actions.SavePermissionProfile(ctx, contracts.PermissionProfile{
		ProfileID: "no-approval", DisplayName: "Cannot approve", Agents: []string{"builder-1"},
		DenyActions:   []contracts.PermissionAction{contracts.PermissionActionGateApprove},
		SchemaVersion: contracts.CurrentSchemaVersion,
	}, "human:owner", "separate reviewer duty"); err != nil {
		t.Fatal(err)
	}
	ticket := scheduleTestTicket("APP-1", contracts.StatusInReview, now)
	ticket.Assignee, ticket.Reviewer = "agent:builder-1", "agent:builder-1"
	ticket.ReviewState = contracts.ReviewStatePending
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.ApproveTicket(ctx, ticket.ID, ticket.Reviewer, "approve own work"); apperr.CodeOf(err) != apperr.CodePermissionDenied {
		t.Fatalf("assigned reviewer bypassed permission profile: %v", err)
	}
	after, err := tickets.GetTicket(ctx, ticket.ID)
	if err != nil || after.ReviewState != contracts.ReviewStatePending || after.Status != contracts.StatusInReview {
		t.Fatalf("denied approval changed ticket: %#v %v", after, err)
	}
	if cfg, err := config.Load(root); err != nil || cfg.Workflow.CompletionMode != contracts.CompletionModeOpen {
		t.Fatalf("test must independently exercise permissions under open completion: %#v %v", cfg, err)
	}
}

func TestReviewTeamPresetDryRunAndExplicitPolicyOverrides(t *testing.T) {
	root, ctx, actions := setupTeamPresetTest(t)
	for _, project := range []contracts.Project{
		{Key: "OLD", Name: "Legacy", CreatedAt: actions.now(), Defaults: contracts.ProjectDefaults{CompletionMode: contracts.CompletionModeOpen}},
		{Key: "STRICT", Name: "Strict", CreatedAt: actions.now(), Defaults: contracts.ProjectDefaults{CompletionMode: contracts.CompletionModeDualGate, RequiredReviewer: "human:reviewer"}},
	} {
		if err := actions.CreateProject(ctx, project); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := actions.ApplyTeamPreset(ctx, "pair", "", true, "human:owner", "preview"); err != nil {
		t.Fatal(err)
	}
	legacy, err := actions.Projects.GetProject(ctx, "OLD")
	if err != nil || legacy.Defaults.CompletionMode != contracts.CompletionModeOpen {
		t.Fatalf("dry run changed project: %#v %v", legacy, err)
	}
	if cfg, err := config.Load(root); err != nil || cfg.Workflow.CompletionMode != contracts.CompletionModeOpen || cfg.Workflow.RequiredReviewer != "" {
		t.Fatalf("dry run changed workflow: %#v %v", cfg, err)
	}
	if _, err := actions.ApplyTeamPreset(ctx, "pair", "", false, "human:owner", "require review"); err != nil {
		t.Fatal(err)
	}
	legacy, err = actions.Projects.GetProject(ctx, "OLD")
	if err != nil || legacy.Defaults.CompletionMode != "" {
		t.Fatalf("legacy open override retained: %#v %v", legacy, err)
	}
	strict, err := actions.Projects.GetProject(ctx, "STRICT")
	if err != nil || strict.Defaults.CompletionMode != contracts.CompletionModeDualGate || strict.Defaults.RequiredReviewer != "human:reviewer" {
		t.Fatalf("explicit policy changed: %#v %v", strict, err)
	}
}

func TestReviewClaimStillBelongsToReviewer(t *testing.T) {
	_, ctx, actions, _, tickets, now := setupScheduleTest(t, nil)
	ticket := scheduleTestTicket("APP-1", contracts.StatusInReview, now)
	ticket.Assignee, ticket.Reviewer = "agent:builder-1", "agent:reviewer-1"
	ticket.ReviewState = contracts.ReviewStatePending
	if err := tickets.CreateTicket(ctx, ticket); err != nil {
		t.Fatal(err)
	}
	claimed, err := actions.ClaimTicket(ctx, ticket.ID, ticket.Reviewer, "review assigned work")
	if err != nil || claimed.Lease.Kind != contracts.LeaseKindReview {
		t.Fatalf("review claim: %#v %v", claimed.Lease, err)
	}
}

func TestScheduleRejectsNonfutureTimeWithoutChangingTicket(t *testing.T) {
	for _, offset := range []time.Duration{-time.Hour, 0} {
		t.Run(offset.String(), func(t *testing.T) {
			_, ctx, actions, _, tickets, now := setupScheduleTest(t, nil)
			ticket := scheduleTestTicket("APP-1", contracts.StatusReady, now)
			ticket.Assignee = "human:original"
			if err := tickets.CreateTicket(ctx, ticket); err != nil {
				t.Fatal(err)
			}
			_, err := actions.SetTicketSchedule(ctx, ticket.ID, now.Add(offset), "human:replacement", "human:owner", "schedule work")
			if apperr.CodeOf(err) != apperr.CodeInvalidInput {
				t.Fatalf("nonfuture schedule must be invalid input, got %v", err)
			}
			after, err := tickets.GetTicket(ctx, ticket.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Schedule != nil || after.Assignee != ticket.Assignee {
				t.Fatalf("rejected schedule changed ticket: %#v", after)
			}
		})
	}
}

func TestReviewTeamPresetRequiresTheActualReviewLifecycle(t *testing.T) {
	for _, preset := range []string{"pair", "crossfire"} {
		for _, projectFirst := range []bool{true, false} {
			name := preset + "/project-after-preset"
			if projectFirst {
				name = preset + "/project-before-preset"
			}
			t.Run(name, func(t *testing.T) {
				_, ctx, actions := setupTeamPresetTest(t)
				project := contracts.Project{Key: "APP", Name: "App", CreatedAt: actions.now()}
				if projectFirst {
					project.Defaults.CompletionMode = contracts.CompletionModeOpen
					if err := actions.CreateProject(ctx, project); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := actions.ApplyTeamPreset(ctx, preset, "", false, "human:owner", "require independent review"); err != nil {
					t.Fatal(err)
				}
				if !projectFirst {
					if err := actions.CreateProject(ctx, project); err != nil {
						t.Fatal(err)
					}
				}
				ticket := scheduleTestTicket("APP-1", contracts.StatusInProgress, actions.now())
				ticket.Assignee = "agent:builder-1"
				if err := actions.CreateTicket(ctx, ticket); err != nil {
					t.Fatal(err)
				}
				for _, actor := range []contracts.Actor{"human:owner", "agent:reviewer-1"} {
					if _, err := actions.CompleteTicket(ctx, ticket.ID, actor, "skip review"); err == nil {
						t.Fatalf("%s bypassed %s review from in_progress", actor, preset)
					}
				}
				requested, err := actions.RequestReview(ctx, ticket.ID, ticket.Assignee, "implementation ready")
				if err != nil {
					t.Fatal(err)
				}
				if requested.Reviewer != "agent:reviewer-1" {
					t.Fatalf("team reviewer was not selected: %q", requested.Reviewer)
				}
				if _, err := actions.ApproveTicket(ctx, ticket.ID, ticket.Assignee, "self approval"); err == nil {
					t.Fatal("preset builder approved its own work")
				}
				approved, err := actions.ApproveTicket(ctx, ticket.ID, "agent:reviewer-1", "independent review passed")
				if err != nil || approved.Status != contracts.StatusDone {
					t.Fatalf("reviewer could not complete the review gate: %#v %v", approved, err)
				}
			})
		}
	}
}
