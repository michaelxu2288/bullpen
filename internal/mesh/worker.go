package mesh

import (
	"context"
	"fmt"
	"time"
)

// ProgressFunc lets a handler report how far along it is. Fraction is 0..1;
// note is a short line about what the agent is doing right now.
type ProgressFunc func(fraction float64, note string)

// Worker is a crew member: it heartbeats to the master, listens for
// assignments on its inbox, executes them, and reports results on the bus.
type Worker struct {
	Node  Node
	Bus   *Bus
	inbox <-chan Message

	// Handle executes one assignment. Kept for callers that do not report
	// progress.
	Handle func(ctx context.Context, m Message) (string, error)
	// HandleWithProgress takes precedence over Handle when set, and receives a
	// reporter that publishes MsgProgress on the worker's behalf.
	HandleWithProgress func(ctx context.Context, m Message, report ProgressFunc) (string, error)
}

func NewWorker(node Node, bus *Bus) *Worker {
	node.Role = RoleWorker
	if node.MaxInFlight <= 0 {
		node.MaxInFlight = 2
	}
	return &Worker{Node: node, Bus: bus, inbox: bus.Subscribe(node.ID)}
}

// Run blocks until ctx is cancelled, beating its heart and handling work.
//
// Assignments run concurrently up to MaxInFlight so a worker with capacity two
// really does hold two cards in RUNNING at once. When ctx is cancelled the
// worker goes silent rather than reporting failures: a dead process would not
// send an escalation either, and that silence is what the master's SWIM
// detector exists to notice.
func (w *Worker) Run(ctx context.Context, beat time.Duration, masterID string) {
	if beat <= 0 {
		beat = time.Second
	}
	defer w.Bus.Unsubscribe(w.Node.ID)

	hb := time.NewTicker(beat)
	defer hb.Stop()

	slots := make(chan struct{}, w.Node.MaxInFlight)

	for {
		select {
		case <-ctx.Done():
			return
		case <-hb.C:
			w.Bus.Publish(Message{Kind: MsgHeartbeat, From: w.Node.ID, To: masterID})
		case m, open := <-w.inbox:
			if !open {
				return
			}
			if m.Kind != MsgAssign && m.Kind != MsgHandoff {
				continue
			}
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			w.Bus.Publish(Message{Kind: MsgAck, From: w.Node.ID, To: m.From, TaskID: m.TaskID})
			go func(m Message) {
				defer func() { <-slots }()
				w.execute(ctx, m)
			}(m)
		}
	}
}

func (w *Worker) execute(ctx context.Context, m Message) {
	report := func(fraction float64, note string) {
		if ctx.Err() != nil {
			return
		}
		w.Bus.Publish(Message{
			Kind: MsgProgress, From: w.Node.ID, To: m.From, TaskID: m.TaskID,
			Body: note,
			Headers: map[string]string{
				"progress": fmt.Sprintf("%.3f", clamp01(fraction)),
			},
		})
	}

	var (
		out string
		err error
	)
	switch {
	case w.HandleWithProgress != nil:
		out, err = w.HandleWithProgress(ctx, m, report)
	case w.Handle != nil:
		out, err = w.Handle(ctx, m)
	}

	// dead means silent; the master's failure detector handles the rest
	if ctx.Err() != nil {
		return
	}

	kind := MsgResult
	if err != nil {
		kind, out = MsgEscalate, fmt.Sprintf("error: %v", err)
	}
	w.Bus.Publish(Message{Kind: kind, From: w.Node.ID, To: m.From, TaskID: m.TaskID, Body: out})
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
