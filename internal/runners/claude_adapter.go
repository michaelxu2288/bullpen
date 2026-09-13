package runners

import (
	"context"
	"fmt"
)

type ClaudeAdapter struct{}

func (c ClaudeAdapter) Name() string { return "claude" }

func (c ClaudeAdapter) Complete(ctx context.Context, ac AgentContext, p Prompt) (Response, error) {
	_ = ctx
	prefix := fmt.Sprintf("[claude:%s:%s]", ac.Role, ac.SessionName)
	return Response{
		Output:       prefix + " " + p.Input,
		TokensIn:     len(p.Input) / 3,
		TokensOut:    len(p.Input) / 4,
		Confidence:   0.88,
		SuggestedNext: []string{"run_tests", "open_diff"},
	}, nil
}

func (c ClaudeAdapter) CanUseTool(tool string) bool {
	switch tool {
	case "pinecone.query", "slack.search", "git.diff", "workflow.handoff":
		return true
	default:
		return false
	}
}

func (c ClaudeAdapter) SupportsRole(role Role) bool {
	return role == RolePlanner || role == RoleCoder || role == RoleReviewer
}
