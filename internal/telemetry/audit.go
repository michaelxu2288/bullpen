package telemetry

import (
	"fmt"
	"time"
)

type AuditEntry struct {
	Actor     string
	Action    string
	Target    string
	Result    string
	Timestamp time.Time
}

type Auditor struct {
	entries []AuditEntry
}

func (a *Auditor) Record(actor, action, target, result string) {
	a.entries = append(a.entries, AuditEntry{
		Actor:     actor,
		Action:    action,
		Target:    target,
		Result:    result,
		Timestamp: time.Now(),
	})
}

func (a *Auditor) Render() string {
	out := ""
	for _, e := range a.entries {
		out += fmt.Sprintf("%s | %s | %s | %s | %s\n", e.Timestamp.Format(time.RFC3339), e.Actor, e.Action, e.Target, e.Result)
	}
	return out
}
