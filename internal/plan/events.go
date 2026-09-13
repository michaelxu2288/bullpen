package plan

import (
	"sync"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

// subscriberBuffer is how many events a slow consumer may fall behind before it
// starts dropping. The dashboard only renders a tail, so dropping beats blocking
// the orchestrator on a stalled browser.
const subscriberBuffer = 64

type EventBus struct {
	mu          sync.RWMutex
	history     []domain.Event
	subscribers map[int]chan domain.Event
	nextSubID   int
}

func NewEventBus() *EventBus {
	return &EventBus{
		history:     make([]domain.Event, 0, 256),
		subscribers: map[int]chan domain.Event{},
	}
}

func (e *EventBus) Publish(ev domain.Event) {
	e.mu.Lock()
	e.history = append(e.history, ev)
	targets := make([]chan domain.Event, 0, len(e.subscribers))
	for _, ch := range e.subscribers {
		targets = append(targets, ch)
	}
	e.mu.Unlock()

	for _, ch := range targets {
		select {
		case ch <- ev:
		default:
			// subscriber is behind; drop rather than stall the run
		}
	}
}

func (e *EventBus) History() []domain.Event {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]domain.Event, len(e.history))
	copy(out, e.history)
	return out
}

// Subscribe returns a channel of future events plus a cancel func. Used by the
// dashboard's SSE endpoint.
func (e *EventBus) Subscribe() (<-chan domain.Event, func()) {
	e.mu.Lock()
	defer e.mu.Unlock()

	id := e.nextSubID
	e.nextSubID++
	ch := make(chan domain.Event, subscriberBuffer)
	e.subscribers[id] = ch

	return ch, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if existing, ok := e.subscribers[id]; ok {
			delete(e.subscribers, id)
			close(existing)
		}
	}
}

// SubscriberCount is exposed for the dashboard status strip.
func (e *EventBus) SubscriberCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.subscribers)
}
