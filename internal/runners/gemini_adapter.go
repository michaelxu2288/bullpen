package runners

import "context"

type GeminiAdapter struct{}

func (g GeminiAdapter) Name() string { return "gemini" }

func (g GeminiAdapter) Complete(ctx context.Context, ac AgentContext, p Prompt) (Response, error) {
	_ = ctx
	_ = ac
	return Response{
		Output:       "[gemini] " + p.Input,
		TokensIn:     len(p.Input) / 2,
		TokensOut:    len(p.Input) / 5,
		Confidence:   0.80,
		SuggestedNext: []string{"summarize"},
	}, nil
}

func (g GeminiAdapter) CanUseTool(tool string) bool {
	return tool == "slack.search" || tool == "pinecone.query"
}

func (g GeminiAdapter) SupportsRole(role Role) bool {
	return role == RolePlanner || role == RoleReviewer
}
