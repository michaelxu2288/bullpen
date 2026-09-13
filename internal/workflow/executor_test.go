package workflow

import "testing"

func TestTemplateExec(t *testing.T) {
	g := ClaudeCodexReviewTemplate()
	exec := Executor{}
	res, err := exec.Execute(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Steps) == 0 {
		t.Fatalf("expected steps")
	}
	if res.Workflow == "" {
		t.Fatalf("expected workflow name")
	}
}
