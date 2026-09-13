package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	ProjectName        string             `json:"project_name"`
	DefaultProvider    string             `json:"default_provider"`
	BranchPrefix       string             `json:"branch_prefix"`
	EnableAutoHandoff  bool               `json:"enable_auto_handoff"`
	MaxConcurrentTasks int                `json:"max_concurrent_tasks"`
	Profiles           []ProviderProfile  `json:"profiles"`
	Review             ReviewConfig       `json:"review"`
	Telemetry          TelemetryConfig    `json:"telemetry"`
}

type ReviewConfig struct {
	RequireHumanApproval bool          `json:"require_human_approval"`
	AutoApproveThreshold float64       `json:"auto_approve_threshold"`
	EscalationTimeout    time.Duration `json:"escalation_timeout"`
}

type TelemetryConfig struct {
	LogLevel         string `json:"log_level"`
	EnableTraceFiles bool   `json:"enable_trace_files"`
	TraceDir         string `json:"trace_dir"`
}

func DefaultConfig() Config {
	return Config{
		ProjectName:       "bullpen",
		DefaultProvider:   "claude",
		BranchPrefix:      "agent/",
		EnableAutoHandoff: true,
		MaxConcurrentTasks: 8,
		Profiles: []ProviderProfile{
			{Name: "claude", Program: "claude", Provider: "claude", Temperature: 0.2},
			{Name: "codex", Program: "codex", Provider: "codex", Temperature: 0.2},
			{Name: "aider", Program: "aider", Provider: "aider", Temperature: 0.3},
			{Name: "gemini", Program: "gemini", Provider: "gemini", Temperature: 0.25},
		},
		Review: ReviewConfig{
			RequireHumanApproval: true,
			AutoApproveThreshold: 0.90,
			EscalationTimeout:    10 * time.Minute,
		},
		Telemetry: TelemetryConfig{
			LogLevel:         "info",
			EnableTraceFiles: true,
			TraceDir:         "./.cc-agent-traces",
		},
	}
}

func ConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve config dir: %w", err)
	}
	root := filepath.Join(configDir, "bullpen")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("failed to create config root: %w", err)
	}
	return filepath.Join(root, "config.json"), nil
}

func Load() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := Save(cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}
