package memory

import (
	"testing"
	"time"
)

func TestContextBuild(t *testing.T) {
	cw := ContextWindow{MaxChars: 30}
	messages := []Message{
		{Role: "user", Content: "hello", Timestamp: time.Now()},
		{Role: "assistant", Content: "world", Timestamp: time.Now()},
	}
	out := cw.Build(messages)
	if out == "" {
		t.Fatalf("expected non-empty context")
	}
}
