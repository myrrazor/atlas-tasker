package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
)

func TestSubscriptionStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := SubscriptionStore{Root: root}
	subscription := contracts.Subscription{
		Actor:      contracts.Actor("agent:builder-1"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
		EventTypes: []contracts.EventType{contracts.EventTicketReviewRequested},
	}
	if err := store.SaveSubscription(subscription); err != nil {
		t.Fatalf("save subscription: %v", err)
	}
	items, err := store.ListSubscriptions()
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(items) != 1 || items[0].Target != "APP-1" {
		t.Fatalf("unexpected subscriptions: %#v", items)
	}
	if err := store.DeleteSubscription(subscription); err != nil {
		t.Fatalf("delete subscription: %v", err)
	}
	items, err = store.ListSubscriptions()
	if err != nil {
		t.Fatalf("list subscriptions after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty subscriptions after delete, got %#v", items)
	}
}

func TestSubscriptionStoreKeepsDistinctSanitizedNames(t *testing.T) {
	root := t.TempDir()
	store := SubscriptionStore{Root: root}
	left := contracts.Subscription{
		Actor:      contracts.Actor("agent:foo_bar"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
	}
	right := contracts.Subscription{
		Actor:      contracts.Actor("agent:foo-bar"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
	}
	if err := store.SaveSubscription(left); err != nil {
		t.Fatalf("save left subscription: %v", err)
	}
	if err := store.SaveSubscription(right); err != nil {
		t.Fatalf("save right subscription: %v", err)
	}
	items, err := store.ListSubscriptions()
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected distinct subscription files, got %#v", items)
	}
}

func TestSubscriptionStoreSkipsMalformedFiles(t *testing.T) {
	root := t.TempDir()
	store := SubscriptionStore{Root: root}
	if err := os.MkdirAll(storage.SubscriptionsDir(root), 0o755); err != nil {
		t.Fatalf("create subscriptions dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storage.SubscriptionsDir(root), "broken.toml"), []byte("actor = "), 0o644); err != nil {
		t.Fatalf("write broken subscription: %v", err)
	}
	if err := store.SaveSubscription(contracts.Subscription{
		Actor:      contracts.Actor("agent:builder-1"),
		TargetKind: contracts.SubscriptionTargetTicket,
		Target:     "APP-1",
	}); err != nil {
		t.Fatalf("save valid subscription: %v", err)
	}
	items, err := store.ListSubscriptions()
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(items) != 1 || items[0].Target != "APP-1" {
		t.Fatalf("expected valid subscription to survive malformed sibling file, got %#v", items)
	}
}
