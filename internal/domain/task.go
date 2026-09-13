package domain

import "time"

type TaskState string

const (
	TaskQueued    TaskState = "queued"
	TaskRunning   TaskState = "running"
	TaskBlocked   TaskState = "blocked"
	TaskReviewing TaskState = "reviewing"
	TaskDone      TaskState = "done"
	TaskFailed    TaskState = "failed"
)

type Task struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	State            TaskState         `json:"state"`
	Priority         int               `json:"priority"`
	OwnerSession     string            `json:"owner_session"`
	ReviewerSession  string            `json:"reviewer_session"`
	LinkedTasks      []string          `json:"linked_tasks"`
	Labels           []string          `json:"labels"`
	Metadata         map[string]string `json:"metadata"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	RequiresApproval bool              `json:"requires_approval"`
}

func NewTask(id, title, description string) Task {
	now := time.Now()
	return Task{
		ID:          id,
		Title:       title,
		Description: description,
		State:       TaskQueued,
		Priority:    3,
		Metadata:    map[string]string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
