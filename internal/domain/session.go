package domain

import "time"

type SessionStatus string

const (
	SessionBooting SessionStatus = "booting"
	SessionReady   SessionStatus = "ready"
	SessionBusy    SessionStatus = "busy"
	SessionPaused  SessionStatus = "paused"
	SessionDead    SessionStatus = "dead"
)

type SessionSnapshot struct {
	Name          string            `json:"name"`
	Provider      string            `json:"provider"`
	Program       string            `json:"program"`
	Branch        string            `json:"branch"`
	WorktreePath  string            `json:"worktree_path"`
	Status        SessionStatus     `json:"status"`
	LastOutputSHA string            `json:"last_output_sha"`
	Capabilities  []string          `json:"capabilities"`
	Metadata      map[string]string `json:"metadata"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}
