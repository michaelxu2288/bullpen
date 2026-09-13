package review

import "time"

type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReject  Decision = "reject"
	DecisionRevise  Decision = "revise"
)

type ReviewRequest struct {
	TaskID      string
	Session     string
	Summary     string
	DiffPreview string
	Confidence  float64
	CreatedAt   time.Time
}

type ReviewResponse struct {
	TaskID    string
	Decision  Decision
	Comment   string
	Reviewer  string
	Timestamp time.Time
}

type HITLGate struct {
	Threshold float64
}

func (h HITLGate) NeedsHuman(confidence float64) bool {
	if h.Threshold <= 0 {
		h.Threshold = 0.90
	}
	return confidence < h.Threshold
}
