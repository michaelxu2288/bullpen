package httpapi

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

// lanes is the board's column order, matching the Bubble Tea TUI.
var lanes = []domain.TaskState{
	domain.TaskQueued,
	domain.TaskRunning,
	domain.TaskReviewing,
	domain.TaskDone,
}

var laneTitles = map[domain.TaskState]string{
	domain.TaskQueued:    "BACKLOG",
	domain.TaskRunning:   "RUNNING",
	domain.TaskReviewing: "REVIEW",
	domain.TaskDone:      "DONE",
}

// BoardCard is the browser projection of a task.
type BoardCard struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	State    domain.TaskState `json:"state"`
	Owner    string           `json:"owner"`
	Reviewer string           `json:"reviewer"`
	Priority int              `json:"priority"`
	Labels   []string         `json:"labels"`
	// Progress is 0..1 while an agent holds the card, and -1 when it does not apply.
	Progress float64 `json:"progress"`
	// Note is what the owning agent last said it was doing.
	Note             string    `json:"note,omitempty"`
	RequiresApproval bool      `json:"requires_approval"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Attempts counts pickups. Two is normal: once to code, once to review.
	Attempts int `json:"attempts"`
	// Requeues counts how many times the card bounced back to BACKLOG, from an
	// escalation or a dead worker. This is the number worth a badge.
	Requeues int `json:"requeues"`
}

type BoardLane struct {
	State domain.TaskState `json:"state"`
	Title string           `json:"title"`
	Cards []BoardCard      `json:"cards"`
}

type Board struct {
	Lanes     []BoardLane `json:"lanes"`
	Total     int         `json:"total"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// BoardStore holds the live board. It is a projection: the crew bus is the
// source of truth, and the live run writes every state change here. The
// dashboard reads it and may advance cards by hand.
type BoardStore struct {
	mu        sync.RWMutex
	tasks     map[string]domain.Task
	order     []string
	progress  map[string]float64
	notes     map[string]string
	attempts  map[string]int
	requeues  map[string]int
	updatedAt time.Time

	// Changed fires on every mutation, so the SSE stream can push a snapshot
	// instead of the dashboard polling for it.
	Changed *Notifier
}

func NewBoardStore() *BoardStore {
	return &BoardStore{
		tasks:     map[string]domain.Task{},
		progress:  map[string]float64{},
		notes:     map[string]string{},
		attempts:  map[string]int{},
		requeues:  map[string]int{},
		updatedAt: time.Now(),
		Changed:   NewNotifier(),
	}
}

func (b *BoardStore) touch() {
	b.updatedAt = time.Now()
	if b.Changed != nil {
		b.Changed.Notify()
	}
}

// Add appends tasks to the board without disturbing what is already there.
func (b *BoardStore) Add(tasks []domain.Task) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, task := range tasks {
		if _, exists := b.tasks[task.ID]; !exists {
			b.order = append(b.order, task.ID)
		}
		b.tasks[task.ID] = task
	}
	b.touch()
}

// Assign records that an agent picked the card up and which lane that puts it in.
func (b *BoardStore) Assign(id, owner string, state domain.TaskState) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	task, ok := b.tasks[id]
	if !ok {
		return fmt.Errorf("card not found: %s", id)
	}
	task.OwnerSession = owner
	task.State = state
	task.UpdatedAt = time.Now()
	b.tasks[id] = task
	b.progress[id] = 0
	b.notes[id] = "picked up"
	b.attempts[id]++
	b.touch()
	return nil
}

// SetProgress records a worker's progress report.
func (b *BoardStore) SetProgress(id string, fraction float64, note string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.tasks[id]; !ok {
		return
	}
	b.progress[id] = fraction
	if note != "" {
		b.notes[id] = note
	}
	b.touch()
}

// AnnotateIfHeld sets a card's note only if it is still held by owner in an
// active lane. The liveness annotator snapshots the board and then writes; the
// reaper can requeue the card in between, and an unconditional write would
// stamp "owner is dead" over the fresh "requeued" note.
func (b *BoardStore) AnnotateIfHeld(id, owner, note string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	task, ok := b.tasks[id]
	if !ok || task.OwnerSession != owner {
		return false
	}
	if task.State != domain.TaskRunning && task.State != domain.TaskReviewing {
		return false
	}
	if b.notes[id] == note {
		return false
	}
	b.notes[id] = note
	b.touch()
	return true
}

// Requeue sends a card back to BACKLOG with no owner. Reason is kept as the
// note so the board says why it bounced.
func (b *BoardStore) Requeue(id, reason string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	task, ok := b.tasks[id]
	if !ok {
		return fmt.Errorf("card not found: %s", id)
	}
	task.State = domain.TaskQueued
	task.OwnerSession = ""
	task.UpdatedAt = time.Now()
	b.tasks[id] = task
	delete(b.progress, id)
	b.notes[id] = reason
	b.requeues[id]++
	b.touch()
	return nil
}

// Counts returns how many cards sit in each lane.
func (b *BoardStore) Counts() map[domain.TaskState]int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := map[domain.TaskState]int{}
	for _, task := range b.tasks {
		out[task.State]++
	}
	return out
}

// Active reports whether any card is still short of DONE.
func (b *BoardStore) Active() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, task := range b.tasks {
		if task.State != domain.TaskDone && task.State != domain.TaskFailed {
			return true
		}
	}
	return false
}

// ReplaceWithOwners swaps in fresh tasks and stamps each one with the session the
// router assigned it, which the planner does not know about.
func (b *BoardStore) ReplaceWithOwners(tasks []domain.Task, sessionByTask map[string]string) {
	owned := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if session, ok := sessionByTask[task.ID]; ok && session != "" {
			task.OwnerSession = session
		}
		owned = append(owned, task)
	}
	b.Replace(owned)
}

// Replace swaps in a fresh set of tasks, preserving the given order.
func (b *BoardStore) Replace(tasks []domain.Task) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tasks = make(map[string]domain.Task, len(tasks))
	b.order = make([]string, 0, len(tasks))
	b.progress = map[string]float64{}
	b.notes = map[string]string{}
	b.attempts = map[string]int{}
	b.requeues = map[string]int{}
	for _, task := range tasks {
		b.tasks[task.ID] = task
		b.order = append(b.order, task.ID)
	}
	b.touch()
}

// Snapshot renders the board grouped into lanes.
func (b *BoardStore) Snapshot() Board {
	b.mu.RLock()
	defer b.mu.RUnlock()

	byState := map[domain.TaskState][]BoardCard{}
	for _, id := range b.order {
		task, ok := b.tasks[id]
		if !ok {
			continue
		}
		byState[task.State] = append(byState[task.State], b.card(task))
	}

	board := Board{Lanes: make([]BoardLane, 0, len(lanes)), Total: len(b.order), UpdatedAt: b.updatedAt}
	for _, state := range lanes {
		cards := byState[state]
		// highest priority first, then most recently touched
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].Priority != cards[j].Priority {
				return cards[i].Priority < cards[j].Priority
			}
			return cards[i].UpdatedAt.After(cards[j].UpdatedAt)
		})
		board.Lanes = append(board.Lanes, BoardLane{State: state, Title: laneTitles[state], Cards: cards})
	}
	return board
}

// Advance moves a card one lane to the right and returns the new state.
func (b *BoardStore) Advance(id string) (domain.Task, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	task, ok := b.tasks[id]
	if !ok {
		return domain.Task{}, fmt.Errorf("card not found: %s", id)
	}

	next, err := nextLane(task.State)
	if err != nil {
		return domain.Task{}, err
	}
	task.State = next
	task.UpdatedAt = time.Now()
	b.tasks[id] = task
	if next == domain.TaskDone {
		delete(b.progress, id)
		b.notes[id] = "advanced by hand"
	}
	b.touch()
	return task, nil
}

// SetState moves a card to an explicit lane, for drag-style moves.
func (b *BoardStore) SetState(id string, state domain.TaskState) (domain.Task, error) {
	if laneTitles[state] == "" {
		return domain.Task{}, fmt.Errorf("unknown lane: %s", state)
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	task, ok := b.tasks[id]
	if !ok {
		return domain.Task{}, fmt.Errorf("card not found: %s", id)
	}
	task.State = state
	task.UpdatedAt = time.Now()
	b.tasks[id] = task
	b.touch()
	return task, nil
}

func nextLane(current domain.TaskState) (domain.TaskState, error) {
	for i, state := range lanes {
		if state != current {
			continue
		}
		if i == len(lanes)-1 {
			return current, fmt.Errorf("card is already in %s", laneTitles[current])
		}
		return lanes[i+1], nil
	}
	return current, fmt.Errorf("card is not on the board (state %s)", current)
}

// card projects a task plus the store's live progress. Callers hold the lock.
func (b *BoardStore) card(task domain.Task) BoardCard {
	card := BoardCard{
		ID:               task.ID,
		Title:            task.Title,
		State:            task.State,
		Owner:            task.OwnerSession,
		Reviewer:         task.ReviewerSession,
		Priority:         task.Priority,
		Labels:           task.Labels,
		Progress:         -1,
		Note:             b.notes[task.ID],
		RequiresApproval: task.RequiresApproval,
		UpdatedAt:        task.UpdatedAt,
		Attempts:         b.attempts[task.ID],
		Requeues:         b.requeues[task.ID],
	}
	if fraction, ok := b.progress[task.ID]; ok && (task.State == domain.TaskRunning || task.State == domain.TaskReviewing) {
		card.Progress = fraction
	}
	return card
}

// Card returns one card's current projection.
func (b *BoardStore) Card(id string) (BoardCard, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	task, ok := b.tasks[id]
	if !ok {
		return BoardCard{}, false
	}
	return b.card(task), true
}
