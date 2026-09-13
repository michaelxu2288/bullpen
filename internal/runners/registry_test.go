package runners

import "testing"

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(ClaudeAdapter{})
r.Register(CodexAdapter{})

	if _, err := r.Get("claude"); err != nil {
		t.Fatalf("expected claude provider: %v", err)
	}
	if _, err := r.Get("codex"); err != nil {
		t.Fatalf("expected codex provider: %v", err)
	}
	if _, err := r.Get("missing"); err == nil {
		t.Fatalf("expected error for missing provider")
	}
}
