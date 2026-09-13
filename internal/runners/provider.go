package runners

import (
	"context"
	"time"
)

type Role string

const (
	RolePlanner Role = "planner"
	RoleCoder   Role = "coder"
	RoleReviewer Role = "reviewer"
)

type AgentContext struct {
	SessionName string
	Provider    string
	Role        Role
	TaskID      string
	Branch      string
	Worktree    string
	Deadline    time.Time
	Metadata    map[string]string
}

type Prompt struct {
	Input      string
	SystemHint string
	Tools      []string
	TraceID    string
}

type Response struct {
	Output         string
	TokensIn       int
	TokensOut      int
	ToolCalls      []ToolCall
	RequiresHITL   bool
	Confidence     float64
	SuggestedNext  []string
}

type ToolCall struct {
	Name   string
	Params map[string]any
}

type Provider interface {
	Name() string
	Complete(ctx context.Context, ac AgentContext, p Prompt) (Response, error)
	CanUseTool(tool string) bool
	SupportsRole(role Role) bool
}
