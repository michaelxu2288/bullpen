package workflow

func ClaudeCodexReviewTemplate() Graph {
	return Graph{
		Name: "claude-codex-review-loop",
		Nodes: []Node{
			{ID: "plan", Type: NodePrompt, Name: "Plan Task", Config: map[string]string{"session": "planner-claude"}},
			{ID: "retrieve", Type: NodeTool, Name: "RAG Retrieve", Config: map[string]string{"tool": "pinecone.query"}, Depends: []string{"plan"}},
			{ID: "implement", Type: NodePrompt, Name: "Implement in Codex", Config: map[string]string{"session": "coder-codex"}, Depends: []string{"retrieve"}},
			{ID: "review", Type: NodePrompt, Name: "Review with Claude", Config: map[string]string{"session": "reviewer-claude"}, Depends: []string{"implement"}},
			{ID: "hitl", Type: NodeHITL, Name: "Human Approval", Depends: []string{"review"}},
			{ID: "merge_gate", Type: NodeGate, Name: "Merge Gate", Depends: []string{"hitl"}},
		},
	}
}
