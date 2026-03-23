package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/contracts"
	"github.com/myrrazor/atlas-tasker/internal/storage"
	"github.com/pelletier/go-toml/v2"
)

type fileConfig struct {
	Workflow struct {
		CompletionMode string `toml:"completion_mode"`
	} `toml:"workflow"`
	Actor struct {
		Default string `toml:"default"`
	} `toml:"actor"`
	Notifications struct {
		Terminal    *bool  `toml:"terminal"`
		FileEnabled bool   `toml:"file_enabled"`
		FilePath    string `toml:"file_path"`
	} `toml:"notifications"`
}

func defaultConfig() contracts.TrackerConfig {
	return contracts.TrackerConfig{
		Workflow:      contracts.WorkflowConfig{CompletionMode: contracts.CompletionModeOpen},
		Actor:         contracts.ActorConfig{},
		Notifications: contracts.NotificationsConfig{Terminal: true, FilePath: filepath.Join(storage.TrackerDir(""), "notifications.log")},
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
			FileEnabled: parsed.Notifications.FileEnabled,
			FilePath:    strings.TrimSpace(parsed.Notifications.FilePath),
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
	if cfg.Notifications.FilePath == "" {
		cfg.Notifications.FilePath = filepath.Join(storage.TrackerDir(root), "notifications.log")
	}
	if err := cfg.Validate(); err != nil {
		return contracts.TrackerConfig{}, err
	}
	return cfg, nil
}

func Save(root string, cfg contracts.TrackerConfig) error {
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
	default:
		return fmt.Errorf("unsupported config key: %s", key)
	}
	return Save(root, cfg)
}
