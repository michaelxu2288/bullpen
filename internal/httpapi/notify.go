package httpapi

import "sync"

// Notifier is a coalescing change signal. Subscribers get at most one pending
// wake-up no matter how many mutations land between reads, which is what the
// SSE board push wants: "something changed, send a snapshot", not one frame per
// progress tick.
type Notifier struct {
	mu   sync.Mutex
	subs map[int]chan struct{}
	next int
}

func NewNotifier() *Notifier {
	return &Notifier{subs: make(map[int]chan struct{})}
}

func (n *Notifier) Subscribe() (<-chan struct{}, func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	id := n.next
	n.next++
	ch := make(chan struct{}, 1)
	n.subs[id] = ch
	return ch, func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if existing, ok := n.subs[id]; ok {
			delete(n.subs, id)
			close(existing)
		}
	}
}

func (n *Notifier) Notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, ch := range n.subs {
		select {
		case ch <- struct{}{}:
		default:
			// already has a pending wake-up; coalesce
		}
	}
}
