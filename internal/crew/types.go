package crew

import "time"

type Session struct {
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Program     string    `json:"program"`
	Branch      string    `json:"branch"`
	Worktree    string    `json:"worktree"`
	TmuxSession string    `json:"tmux_session"`
	CreatedAt   time.Time `json:"created_at"`
}

type LaunchOptions struct {
	Name         string
	Provider     string
	Program      string
	BranchPrefix string
}
