package contracts

import "testing"

func TestSubscriptionValidate(t *testing.T) {
	valid := Subscription{
		Actor:      Actor("agent:builder-1"),
		TargetKind: SubscriptionTargetTicket,
		Target:     "APP-1",
		EventTypes: []EventType{EventTicketReviewRequested},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid subscription, got %v", err)
	}

	cases := []Subscription{
		{Actor: "", TargetKind: SubscriptionTargetTicket, Target: "APP-1"},
		{Actor: Actor("agent:builder-1"), TargetKind: SubscriptionTargetKind("wat"), Target: "APP-1"},
		{Actor: Actor("agent:builder-1"), TargetKind: SubscriptionTargetTicket, Target: ""},
		{Actor: Actor("agent:builder-1"), TargetKind: SubscriptionTargetTicket, Target: "APP-1", EventTypes: []EventType{"wat"}},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected validation failure for %#v", tc)
		}
	}
}
