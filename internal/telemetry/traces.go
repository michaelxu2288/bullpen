package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type TraceSpan struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	ParentSpan string            `json:"parent_span"`
	Name       string            `json:"name"`
	Attrs      map[string]string `json:"attrs"`
	StartedAt  time.Time         `json:"started_at"`
	EndedAt    time.Time         `json:"ended_at"`
}

type TraceSink struct {
	Dir string
}

func (t TraceSink) Write(span TraceSpan) error {
	if t.Dir == "" {
		t.Dir = ".cc-agent-traces"
	}
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(t.Dir, span.TraceID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(span)
}
