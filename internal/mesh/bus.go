package mesh

import (
	"context"
	"sync"
	"time"
)

// MsgKind enumerates the internal agent-to-agent protocol verbs that ride the
// gossip bus between the master and worker agents.
type MsgKind string

const (
	MsgHeartbeat MsgKind = "heartbeat"
	MsgAssign    MsgKind = "assign"
	MsgAck       MsgKind = "ack"
	MsgProgress  MsgKind = "progress"
	MsgHandoff   MsgKind = "handoff"
	MsgResult    MsgKind = "result"
	MsgEscalate  MsgKind = "escalate"
	MsgGossip    MsgKind = "gossip"
	MsgBroadcast MsgKind = "broadcast"
)

// Message is the envelope passed on the internal bus. From/To are node IDs;
// To == "*" means broadcast to the whole mesh.
type Message struct {
	ID      string            `json:"id"`
	Kind    MsgKind           `json:"kind"`
	From    string            `json:"from"`
	To      string            `json:"to"`
	TaskID  string            `json:"task_id"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers"`
	TS      time.Time         `json:"ts"`
}

type subscriber struct {
	id string
	ch chan Message
}

// RemoteTransport carries bus messages to agents that live in another process.
// WebhookTransport is the implementation; the interface keeps the bus testable
// and leaves room for a NATS/Redis carrier later.
type RemoteTransport interface {
	Known(nodeID string) bool
	Deliver(ctx context.Context, m Message) error
	Broadcast(ctx context.Context, m Message) []error
}

// Bus is the crew's pub/sub fabric: one inbox per local node, a broadcast
// fan-out, and an optional remote transport for agents in other processes.
//
// Addressing is uniform. A publisher names a node id and never has to know
// whether that agent is a goroutine in this binary or a webhook endpoint on
// another machine; the bus decides.
type Bus struct {
	mu      sync.RWMutex
	subs    map[string]*subscriber
	taps    map[int]chan Message
	nextTap int
	history []Message
	maxHist int

	remote RemoteTransport
	// OnRemoteError observes delivery failures, which happen on a background
	// goroutine and would otherwise be silent.
	OnRemoteError func(error)
}

func NewBus() *Bus {
	return &Bus{subs: make(map[string]*subscriber), taps: make(map[int]chan Message), maxHist: 512}
}

// Tap returns a firehose of every message on the bus regardless of addressee.
// Projections — the kanban board, the audit timeline — need to see all traffic
// without being a node; this is how they do it without polling Tail.
func (b *Bus) Tap() (<-chan Message, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.nextTap
	b.nextTap++
	ch := make(chan Message, 256)
	b.taps[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if existing, ok := b.taps[id]; ok {
			delete(b.taps, id)
			close(existing)
		}
	}
}

// AttachRemote installs the transport used for node ids with no local inbox.
func (b *Bus) AttachRemote(t RemoteTransport) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.remote = t
}

func (b *Bus) reportRemote(err error) {
	if err == nil {
		return
	}
	b.mu.RLock()
	hook := b.OnRemoteError
	b.mu.RUnlock()
	if hook != nil {
		hook(err)
	}
}

// Subscribe registers a node inbox and returns its receive channel.
func (b *Bus) Subscribe(nodeID string) <-chan Message {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &subscriber{id: nodeID, ch: make(chan Message, 64)}
	b.subs[nodeID] = s
	return s.ch
}

func (b *Bus) Unsubscribe(nodeID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.subs[nodeID]; ok {
		close(s.ch)
		delete(b.subs, nodeID)
	}
}

// Publish routes a message to one inbox or fans out to every subscriber.
func (b *Bus) Publish(m Message) {
	if m.TS.IsZero() {
		m.TS = time.Now()
	}
	b.mu.Lock()
	b.history = append(b.history, m)
	if len(b.history) > b.maxHist {
		b.history = b.history[len(b.history)-b.maxHist:]
	}
	targets := make([]*subscriber, 0, len(b.subs))
	deliveredLocally := false
	for _, s := range b.subs {
		if m.To == "*" || m.To == s.id {
			targets = append(targets, s)
			if m.To == s.id {
				deliveredLocally = true
			}
		}
	}
	taps := make([]chan Message, 0, len(b.taps))
	for _, tap := range b.taps {
		taps = append(taps, tap)
	}
	remote := b.remote
	b.mu.Unlock()

	for _, s := range targets {
		select {
		case s.ch <- m:
		default:
			// slow consumer: drop rather than block the crew
		}
	}
	for _, tap := range taps {
		select {
		case tap <- m:
		default:
			// a projection that falls behind loses frames, never the crew
		}
	}

	if remote == nil {
		return
	}

	// Remote delivery is network I/O. It runs off the publish path so a slow or
	// unreachable agent cannot stall the crew, which is the same reason the
	// local fan-out above drops instead of blocking.
	switch {
	case m.To == "*":
		go func() {
			for _, err := range remote.Broadcast(context.Background(), m) {
				b.reportRemote(err)
			}
		}()
	case !deliveredLocally && remote.Known(m.To):
		go func() {
			b.reportRemote(remote.Deliver(context.Background(), m))
		}()
	}
}

// Tail returns the most recent n messages for the audit timeline / TUI.
func (b *Bus) Tail(n int) []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if n > len(b.history) {
		n = len(b.history)
	}
	out := make([]Message, n)
	copy(out, b.history[len(b.history)-n:])
	return out
}
