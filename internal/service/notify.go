package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type Notifier interface {
	Notify(context.Context, contracts.Event) error
}

type notifierFunc func(context.Context, contracts.Event) error

func (fn notifierFunc) Notify(ctx context.Context, event contracts.Event) error {
	return fn(ctx, event)
}

func BuildNotifier(root string, cfg contracts.TrackerConfig, stderr io.Writer) (Notifier, error) {
	notifiers := make([]Notifier, 0, 2)
	if cfg.Notifications.Terminal && stderr != nil {
		notifiers = append(notifiers, notifierFunc(func(_ context.Context, event contracts.Event) error {
			if !shouldNotify(event.Type) {
				return nil
			}
			_, err := fmt.Fprintf(stderr, "[tracker] %s %s %s\n", event.Type, event.TicketID, strings.TrimSpace(event.Reason))
			return err
		}))
	}
	if cfg.Notifications.FileEnabled {
		path := cfg.Notifications.FilePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		notifiers = append(notifiers, notifierFunc(func(_ context.Context, event contracts.Event) error {
			if !shouldNotify(event.Type) {
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			defer file.Close()
			raw, err := json.Marshal(event)
			if err != nil {
				return err
			}
			_, err = file.Write(append(raw, '\n'))
			return err
		}))
	}
	if len(notifiers) == 0 {
		return nil, nil
	}
	return notifierFunc(func(ctx context.Context, event contracts.Event) error {
		for _, notifier := range notifiers {
			if err := notifier.Notify(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}), nil
}

func shouldNotify(kind contracts.EventType) bool {
	switch kind {
	case contracts.EventTicketLeaseExpired,
		contracts.EventTicketReviewRequested,
		contracts.EventTicketApproved,
		contracts.EventTicketRejected,
		contracts.EventTicketPolicyUpdated,
		contracts.EventProjectPolicyUpdated,
		contracts.EventOwnerAttentionRaised,
		contracts.EventOwnerAttentionCleared:
		return true
	default:
		return false
	}
}
