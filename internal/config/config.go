package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

var embeddedURLPattern = regexp.MustCompile(`https?://[^\s'"]+`)

type fileConfig struct {
	Workflow struct {
		CompletionMode string `toml:"completion_mode"`
	} `toml:"workflow"`
	Actor struct {
		Default string `toml:"default"`
	} `toml:"actor"`
	Notifications struct {
		Terminal              *bool  `toml:"terminal"`
		FileEnabled           bool   `toml:"file_enabled"`
		FilePath              string `toml:"file_path"`
		WebhookURL            string `toml:"webhook_url"`
		WebhookTimeoutSeconds int    `toml:"webhook_timeout_seconds"`
		WebhookRetries        int    `toml:"webhook_retries"`
		DeliveryLogPath       string `toml:"delivery_log_path"`
		DeadLetterPath        string `toml:"dead_letter_path"`
	} `toml:"notifications"`
}

func defaultConfig() contracts.TrackerConfig {
	return contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Actor:    contracts.ActorConfig{},
		Notifications: contracts.NotificationsConfig{
			Terminal:              true,
			FilePath:              filepath.Join(storage.TrackerDir(""), "notifications.log"),
			WebhookTimeoutSeconds: 3,
			WebhookRetries:        2,
			DeliveryLogPath:       filepath.Join(storage.TrackerDir(""), "notification-delivery.log"),
			DeadLetterPath:        filepath.Join(storage.TrackerDir(""), "notification-dead-letter.log"),
		},
	}
}

func applyNotificationDefaults(root string, cfg *contracts.TrackerConfig) {
	if cfg.Notifications.FilePath == "" {
		cfg.Notifications.FilePath = filepath.Join(storage.TrackerDir(root), "notifications.log")
	}
	if cfg.Notifications.WebhookTimeoutSeconds == 0 {
		cfg.Notifications.WebhookTimeoutSeconds = 3
	}
	if cfg.Notifications.DeliveryLogPath == "" {
		cfg.Notifications.DeliveryLogPath = filepath.Join(storage.TrackerDir(root), "notification-delivery.log")
	}
	if cfg.Notifications.DeadLetterPath == "" {
		cfg.Notifications.DeadLetterPath = filepath.Join(storage.TrackerDir(root), "notification-dead-letter.log")
	}
}

func configPath(root string) string {
	return filepath.Join(storage.TrackerDir(root), "config.toml")
}

func Load(root string) (contracts.TrackerConfig, error) {
	path := configPath(root)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return contracts.TrackerConfig{}, fmt.Errorf("read config: %w", err)
	}
	var parsed fileConfig
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return contracts.TrackerConfig{}, fmt.Errorf("parse config: %w", err)
	}
	cfg := contracts.TrackerConfig{
		Workflow: contracts.WorkflowConfig{CompletionMode: contracts.CompletionMode(strings.TrimSpace(parsed.Workflow.CompletionMode))},
		Actor: contracts.ActorConfig{
			Default: contracts.Actor(strings.TrimSpace(parsed.Actor.Default)),
		},
		Notifications: contracts.NotificationsConfig{
			FileEnabled:           parsed.Notifications.FileEnabled,
			FilePath:              strings.TrimSpace(parsed.Notifications.FilePath),
			WebhookURL:            strings.TrimSpace(parsed.Notifications.WebhookURL),
			WebhookTimeoutSeconds: parsed.Notifications.WebhookTimeoutSeconds,
			WebhookRetries:        parsed.Notifications.WebhookRetries,
			DeliveryLogPath:       strings.TrimSpace(parsed.Notifications.DeliveryLogPath),
			DeadLetterPath:        strings.TrimSpace(parsed.Notifications.DeadLetterPath),
		},
	}
	if cfg.Workflow.CompletionMode == "" {
		cfg.Workflow.CompletionMode = contracts.CompletionModeOpen
	}
	if parsed.Notifications.Terminal == nil {
		cfg.Notifications.Terminal = true
	} else {
		cfg.Notifications.Terminal = *parsed.Notifications.Terminal
	}
	applyNotificationDefaults(root, &cfg)
	if err := cfg.Validate(); err != nil {
		return contracts.TrackerConfig{}, err
	}
	return cfg, nil
}

func Save(root string, cfg contracts.TrackerConfig) error {
	applyNotificationDefaults(root, &cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}
	path := configPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	out := fileConfig{}
	out.Workflow.CompletionMode = string(cfg.Workflow.CompletionMode)
	out.Actor.Default = string(cfg.Actor.Default)
	out.Notifications.Terminal = &cfg.Notifications.Terminal
	out.Notifications.FileEnabled = cfg.Notifications.FileEnabled
	out.Notifications.FilePath = cfg.Notifications.FilePath
	out.Notifications.WebhookURL = cfg.Notifications.WebhookURL
	out.Notifications.WebhookTimeoutSeconds = cfg.Notifications.WebhookTimeoutSeconds
	out.Notifications.WebhookRetries = cfg.Notifications.WebhookRetries
	out.Notifications.DeliveryLogPath = cfg.Notifications.DeliveryLogPath
	out.Notifications.DeadLetterPath = cfg.Notifications.DeadLetterPath
	raw, err := toml.Marshal(out)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func Get(root string, key string) (string, error) {
	cfg, err := Load(root)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(key) {
	case "", "workflow.completion_mode":
		return string(cfg.Workflow.CompletionMode), nil
	case "actor.default":
		return string(cfg.Actor.Default), nil
	case "notifications.terminal":
		if cfg.Notifications.Terminal {
			return "true", nil
		}
		return "false", nil
	case "notifications.file_enabled":
		if cfg.Notifications.FileEnabled {
			return "true", nil
		}
		return "false", nil
	case "notifications.file_path":
		return cfg.Notifications.FilePath, nil
	case "notifications.webhook_url":
		return MaskSensitiveConfigValue("notifications.webhook_url", cfg.Notifications.WebhookURL), nil
	case "notifications.webhook_timeout_seconds":
		return fmt.Sprintf("%d", cfg.Notifications.WebhookTimeoutSeconds), nil
	case "notifications.webhook_retries":
		return fmt.Sprintf("%d", cfg.Notifications.WebhookRetries), nil
	case "notifications.delivery_log_path":
		return cfg.Notifications.DeliveryLogPath, nil
	case "notifications.dead_letter_path":
		return cfg.Notifications.DeadLetterPath, nil
	default:
		return "", fmt.Errorf("unsupported config key: %s", key)
	}
}

func Set(root string, key string, value string) error {
	cfg, err := Load(root)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(key) {
	case "workflow.completion_mode":
		cfg.Workflow.CompletionMode = contracts.CompletionMode(strings.TrimSpace(value))
	case "actor.default":
		cfg.Actor.Default = contracts.Actor(strings.TrimSpace(value))
	case "notifications.terminal":
		cfg.Notifications.Terminal = strings.EqualFold(strings.TrimSpace(value), "true")
	case "notifications.file_enabled":
		cfg.Notifications.FileEnabled = strings.EqualFold(strings.TrimSpace(value), "true")
	case "notifications.file_path":
		cfg.Notifications.FilePath = strings.TrimSpace(value)
	case "notifications.webhook_url":
		cfg.Notifications.WebhookURL = strings.TrimSpace(value)
	case "notifications.webhook_timeout_seconds":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid notifications.webhook_timeout_seconds: %w", err)
		}
		cfg.Notifications.WebhookTimeoutSeconds = n
	case "notifications.webhook_retries":
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("invalid notifications.webhook_retries: %w", err)
		}
		cfg.Notifications.WebhookRetries = n
	case "notifications.delivery_log_path":
		cfg.Notifications.DeliveryLogPath = strings.TrimSpace(value)
	case "notifications.dead_letter_path":
		cfg.Notifications.DeadLetterPath = strings.TrimSpace(value)
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}
	return Save(root, cfg)
}

func MaskSensitiveConfigValue(key string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if strings.TrimSpace(key) != "notifications.webhook_url" {
		return value
	}
	return maskSensitiveURL(value)
}

func MaskSecretsInText(value string) string {
	return embeddedURLPattern.ReplaceAllStringFunc(value, maskSensitiveURL)
}

func MaskTrackerConfig(cfg contracts.TrackerConfig) contracts.TrackerConfig {
	cfg.Notifications.WebhookURL = MaskSensitiveConfigValue("notifications.webhook_url", cfg.Notifications.WebhookURL)
	return cfg
}

func maskSensitiveURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	if parsed.User != nil {
		user := parsed.User.Username()
		if user == "" {
			user = "***"
		}
		parsed.User = url.UserPassword(user, "***")
	}
	query := parsed.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "key") || strings.Contains(lower, "password") {
			query.Set(key, "***")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
