package service

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

// SubscriptionStore persists watcher rules under the local tracker workspace.
type SubscriptionStore struct {
	Root string
}

// ListSubscriptions returns all watcher rules sorted by actor, target kind, then target.
func (s SubscriptionStore) ListSubscriptions() ([]contracts.Subscription, error) {
	entries, err := os.ReadDir(storage.SubscriptionsDir(s.Root))
	if err != nil {
		if os.IsNotExist(err) {
			return []contracts.Subscription{}, nil
		}
		return nil, fmt.Errorf("read subscriptions dir: %w", err)
	}
	items := make([]contracts.Subscription, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		subscription, err := s.loadPath(filepath.Join(storage.SubscriptionsDir(s.Root), entry.Name()))
		if err != nil {
			continue
		}
		items = append(items, subscription)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Actor != items[j].Actor {
			return items[i].Actor < items[j].Actor
		}
		if items[i].TargetKind != items[j].TargetKind {
			return items[i].TargetKind < items[j].TargetKind
		}
		return items[i].Target < items[j].Target
	})
	return items, nil
}

// SaveSubscription writes or replaces one watcher rule.
func (s SubscriptionStore) SaveSubscription(subscription contracts.Subscription) error {
	if err := subscription.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(storage.SubscriptionsDir(s.Root), 0o755); err != nil {
		return fmt.Errorf("create subscriptions dir: %w", err)
	}
	raw, err := toml.Marshal(subscription)
	if err != nil {
		return fmt.Errorf("encode subscription: %w", err)
	}
	if err := os.WriteFile(s.subscriptionPath(subscription), raw, 0o644); err != nil {
		return fmt.Errorf("write subscription: %w", err)
	}
	return nil
}

// DeleteSubscription removes one watcher rule.
func (s SubscriptionStore) DeleteSubscription(subscription contracts.Subscription) error {
	if err := subscription.Validate(); err != nil {
		return err
	}
	if err := os.Remove(s.subscriptionPath(subscription)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

func (s SubscriptionStore) loadPath(path string) (contracts.Subscription, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return contracts.Subscription{}, fmt.Errorf("read subscription: %w", err)
	}
	var subscription contracts.Subscription
	if err := toml.Unmarshal(raw, &subscription); err != nil {
		return contracts.Subscription{}, fmt.Errorf("parse subscription: %w", err)
	}
	if err := subscription.Validate(); err != nil {
		return contracts.Subscription{}, err
	}
	return subscription, nil
}

func (s SubscriptionStore) subscriptionPath(subscription contracts.Subscription) string {
	name := fmt.Sprintf("%s--%s--%s.toml", sanitizeSubscriptionPart(string(subscription.Actor)), sanitizeSubscriptionPart(string(subscription.TargetKind)), sanitizeSubscriptionPart(subscription.Target))
	return filepath.Join(storage.SubscriptionsDir(s.Root), name)
}

func sanitizeSubscriptionPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "item"
	}
	return url.PathEscape(value)
}
