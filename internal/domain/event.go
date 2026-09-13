package domain

import "time"

type EventType string

const (
	EventTaskCreated       EventType = "task.created"
	EventTaskAssigned      EventType = "task.assigned"
	EventTaskStateChanged  EventType = "task.state_changed"
	EventSessionLaunched   EventType = "session.launched"
	EventPromptPiped       EventType = "prompt.piped"
	EventToolInvoked       EventType = "tool.invoked"
	EventHITLRequested     EventType = "hitl.requested"
	EventHITLApproved      EventType = "hitl.approved"
	EventHITLRejected      EventType = "hitl.rejected"
)

type Event struct {
	ID        string         `json:"id"`
	Type      EventType      `json:"type"`
	Actor     string         `json:"actor"`
	Target    string         `json:"target"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}
