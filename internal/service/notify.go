package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

type Notifier interface {
	Notify(context.Context, contracts.Event) error
}

type notifierFunc func(context.Context, contracts.Event) error

func (fn notifierFunc) Notify(ctx context.Context, event contracts.Event) error {
	return fn(ctx, event)
}

type NotificationDelivery struct {
	Attempt    int               `json:"attempt"`
	Delivered  bool              `json:"delivered"`
	Error      string            `json:"error,omitempty"`
	Event      contracts.Event   `json:"event"`
	Recipients []contracts.Actor `json:"recipients,omitempty"`
	Targets    []string          `json:"targets,omitempty"`
	Sink       string            `json:"sink"`
	Timestamp  time.Time         `json:"timestamp"`
}

type notificationPayload struct {
	Event       contracts.Event   `json:"event"`
	DeliveredAt time.Time         `json:"delivered_at"`
	Recipients  []contracts.Actor `json:"recipients,omitempty"`
	Targets     []string          `json:"targets,omitempty"`
}

type deliverySink struct {
	name    string
	retries int
	deliver func(context.Context, notificationPayload) error
}

type deliveryNotifier struct {
	deadLetterPath string
	logPath        string
	resolver       SubscriptionResolver
	sinks          []deliverySink
}

func BuildNotifier(root string, cfg contracts.TrackerConfig, stderr io.Writer, resolver SubscriptionResolver) (Notifier, error) {
	sinks := make([]deliverySink, 0, 3)
	if cfg.Notifications.Terminal && stderr != nil {
		sinks = append(sinks, deliverySink{
			name: "terminal",
			deliver: func(_ context.Context, payload notificationPayload) error {
				recipients := ""
				if len(payload.Recipients) > 0 {
					recipients = fmt.Sprintf(" -> %s", strings.Join(actorsToStrings(payload.Recipients), ","))
				}
				_, err := fmt.Fprintf(stderr, "[tracker]%s %s %s %s\n", recipients, payload.Event.Type, payload.Event.TicketID, strings.TrimSpace(payload.Event.Reason))
				return err
			},
		})
	}
	if cfg.Notifications.FileEnabled {
		path := resolveNotifyPath(root, cfg.Notifications.FilePath)
		sinks = append(sinks, deliverySink{
			name: "file",
			deliver: func(_ context.Context, payload notificationPayload) error {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return err
				}
				file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return err
				}
				defer file.Close()
				raw, err := json.Marshal(payload.Event)
				if err != nil {
					return err
				}
				_, err = file.Write(append(raw, '\n'))
				return err
			},
		})
	}
	if strings.TrimSpace(cfg.Notifications.WebhookURL) != "" {
		timeout := time.Duration(cfg.Notifications.WebhookTimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		sinks = append(sinks, deliverySink{
			name:    "webhook",
			retries: cfg.Notifications.WebhookRetries,
			deliver: func(ctx context.Context, payload notificationPayload) error {
				raw, err := json.Marshal(payload)
				if err != nil {
					return err
				}
				reqCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.Notifications.WebhookURL, bytes.NewReader(raw))
				if err != nil {
					return err
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return nil
				}
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				return fmt.Errorf("webhook status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			},
		})
	}
	if len(sinks) == 0 {
		return nil, nil
	}
	return deliveryNotifier{
		sinks:          sinks,
		logPath:        resolveNotifyPath(root, cfg.Notifications.DeliveryLogPath),
		deadLetterPath: resolveNotifyPath(root, cfg.Notifications.DeadLetterPath),
		resolver:       resolver,
	}, nil
}

func (n deliveryNotifier) Notify(ctx context.Context, event contracts.Event) error {
	audience, err := n.resolver.Audience(ctx, event)
	if err != nil {
		if !shouldNotify(event.Type) {
			return nil
		}
		audience = SubscriptionAudience{}
	}
	if !shouldDeliver(event.Type, audience) {
		return nil
	}
	payload := notificationPayload{
		Event:       event,
		DeliveredAt: time.Now().UTC(),
		Recipients:  audience.Recipients,
		Targets:     audience.Targets,
	}
	var errs []error
	for _, sink := range n.sinks {
		attempts := sink.retries + 1
		if attempts <= 0 {
			attempts = 1
		}
		var lastErr error
		for attempt := 1; attempt <= attempts; attempt++ {
			lastErr = sink.deliver(ctx, payload)
			record := NotificationDelivery{
				Attempt:    attempt,
				Delivered:  lastErr == nil,
				Error:      errorString(lastErr),
				Event:      event,
				Recipients: audience.Recipients,
				Targets:    audience.Targets,
				Sink:       sink.name,
				Timestamp:  time.Now().UTC(),
			}
			_ = appendNotificationRecord(n.logPath, record)
			if lastErr == nil {
				break
			}
		}
		if lastErr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", sink.name, lastErr))
			_ = appendNotificationRecord(n.deadLetterPath, NotificationDelivery{
				Attempt:    attempts,
				Delivered:  false,
				Error:      lastErr.Error(),
				Event:      event,
				Recipients: audience.Recipients,
				Targets:    audience.Targets,
				Sink:       sink.name,
				Timestamp:  time.Now().UTC(),
			})
		}
	}
	return errors.Join(errs...)
}

func ReadNotificationLog(root string, cfg contracts.TrackerConfig) ([]NotificationDelivery, error) {
	return readNotificationRecords(resolveNotifyPath(root, cfg.Notifications.DeliveryLogPath))
}

func ReadDeadLetters(root string, cfg contracts.TrackerConfig) ([]NotificationDelivery, error) {
	return readNotificationRecords(resolveNotifyPath(root, cfg.Notifications.DeadLetterPath))
}

func readNotificationRecords(path string) ([]NotificationDelivery, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []NotificationDelivery{}, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []NotificationDelivery{}, nil
	}
	records := make([]NotificationDelivery, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record NotificationDelivery
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func appendNotificationRecord(path string, record NotificationDelivery) error {
	if strings.TrimSpace(path) == "" {
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
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = file.Write(append(raw, '\n'))
	return err
}

func resolveNotifyPath(root string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func shouldDeliver(kind contracts.EventType, audience SubscriptionAudience) bool {
	return shouldNotify(kind) || len(audience.Recipients) > 0
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

func actorsToStrings(values []contracts.Actor) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, string(value))
	}
	return items
}
