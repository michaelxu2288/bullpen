package runners

import "context"

type AiderAdapter struct{}

func (a AiderAdapter) Name() string { return "aider" }

func (a AiderAdapter) Complete(ctx context.Context, ac AgentContext, p Prompt) (Response, error) {
	_ = ctx
	_ = ac
	return Response{
		Output:       "[aider] " + p.Input,
		TokensIn:     len(p.Input) / 4,
		TokensOut:    len(p.Input) / 6,
		Confidence:   0.76,
		SuggestedNext: []string{"stage_patch"},
	}, nil
}

func (a AiderAdapter) CanUseTool(tool string) bool {
	return tool == "git.diff" || tool == "workflow.handoff"
}

func (a AiderAdapter) SupportsRole(role Role) bool {
	return role == RoleCoder
}
