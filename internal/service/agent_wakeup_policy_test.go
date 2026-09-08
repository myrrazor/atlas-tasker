package service

import (
	"testing"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

func TestAutoPromotionHonorsWorkEligibilityDenials(t *testing.T) {
	cases := []struct {
		name      string
		reason    string
		configure func(*contracts.TicketSnapshot, *contracts.AgentProfile, time.Time)
	}{
		{"worker policy", AgentWorkReasonPolicyBlocked, func(ticket *contracts.TicketSnapshot, _ *contracts.AgentProfile, _ time.Time) {
			ticket.Policy.AllowedWorkers = []contracts.Actor{"agent:other"}
		}},
		{"foreign lease", AgentWorkReasonClaimedByOther, func(ticket *contracts.TicketSnapshot, _ *contracts.AgentProfile, now time.Time) {
			ticket.Lease = contracts.LeaseState{Actor: "agent:other", Kind: contracts.LeaseKindWork, AcquiredAt: now, LastHeartbeatAt: now, ExpiresAt: now.Add(time.Hour)}
		}},
		{"disabled agent", AgentWorkReasonAgentDisabled, func(_ *contracts.TicketSnapshot, profile *contracts.AgentProfile, _ time.Time) {
			profile.Enabled = false
		}},
		{"missing capability", AgentWorkReasonMissingCapability, func(ticket *contracts.TicketSnapshot, _ *contracts.AgentProfile, _ time.Time) {
			ticket.RequiredCapabilities = []string{"security-review"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, ctx, actions, queries, tickets, now := setupAgentWakeupTest(t)
			blocker := testAgentWorkTicket("APP-1", "Blocker", contracts.StatusInReview, now)
			blocker.ReviewState = contracts.ReviewStateApproved
			dependent := testAgentWorkTicket("APP-2", "Restricted dependent", contracts.StatusBacklog, now)
			dependent.Assignee = "agent:builder-1"
			dependent.BlockedBy = []string{blocker.ID}
			profile := contracts.AgentProfile{AgentID: "builder-1", DisplayName: "Builder", Provider: contracts.AgentProviderCodex, Enabled: true}
			tc.configure(&dependent, &profile, now)
			if err := (AgentStore{Root: root}).SaveAgent(ctx, profile); err != nil {
				t.Fatal(err)
			}
			for _, ticket := range []contracts.TicketSnapshot{blocker, dependent} {
				if err := tickets.CreateTicket(ctx, ticket); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := actions.MoveTicket(ctx, blocker.ID, contracts.StatusDone, "human:owner", "blocker done"); err != nil {
				t.Fatal(err)
			}
			got, err := tickets.GetTicket(ctx, dependent.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != contracts.StatusBacklog {
				t.Fatalf("denied dependent moved to %s", got.Status)
			}
			wakeups, err := queries.AgentWakeups(ctx, "builder-1")
			if err != nil {
				t.Fatal(err)
			}
			if len(wakeups) != 0 {
				t.Fatalf("denied dependent woke agent: %#v", wakeups)
			}
			events, err := actions.Events.StreamEvents(ctx, "APP", 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if event.Type == contracts.EventTicketMoved && event.TicketID == dependent.ID {
					t.Fatalf("denied promotion was audited: %#v", event)
				}
			}
			view, err := queries.AgentWork(ctx, dependent.Assignee)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range view.Available {
				if entry.Ticket.ID == dependent.ID {
					t.Fatalf("denied dependent reported available: %#v", entry)
				}
			}
			found := false
			for _, entry := range view.Pending {
				if entry.Ticket.ID != dependent.ID {
					continue
				}
				for _, reason := range entry.ReasonCodes {
					if reason == tc.reason {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("expected pending reason %s: %#v", tc.reason, view.Pending)
			}
		})
	}
}
