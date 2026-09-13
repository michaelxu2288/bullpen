package runners

import "context"

type CodexAdapter struct{}

func (c CodexAdapter) Name() string { return "codex" }

func (c CodexAdapter) Complete(ctx context.Context, ac AgentContext, p Prompt) (Response, error) {
	_ = ctx
	_ = ac
	return Response{
		Output:       "[codex] " + p.Input,
		TokensIn:     len(p.Input) / 3,
		TokensOut:    len(p.Input) / 5,
		Confidence:   0.84,
		SuggestedNext: []string{"compile", "run_lint"},
	}, nil
}

func (c CodexAdapter) CanUseTool(tool string) bool {
	return tool == "git.diff" || tool == "workflow.handoff" || tool == "pinecone.query"
}

func (c CodexAdapter) SupportsRole(role Role) bool {
	return role == RoleCoder || role == RoleReviewer
}
