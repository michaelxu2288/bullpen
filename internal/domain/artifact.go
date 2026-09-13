package domain

import "time"

type ArtifactType string

const (
	ArtifactPatch      ArtifactType = "patch"
	ArtifactSummary    ArtifactType = "summary"
	ArtifactReviewNote ArtifactType = "review_note"
	ArtifactToolOutput ArtifactType = "tool_output"
)

type Artifact struct {
	ID           string            `json:"id"`
	TaskID       string            `json:"task_id"`
	SessionName  string            `json:"session_name"`
	Type         ArtifactType      `json:"type"`
	Content      string            `json:"content"`
	ContentHash  string            `json:"content_hash"`
	Metadata     map[string]string `json:"metadata"`
	CreatedAt    time.Time         `json:"created_at"`
}
