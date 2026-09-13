package plan

import "sync"

type HandoffMessage struct {
	From    string
	To      string
	TaskID  string
	Mode    string
	Payload string
}

type HandoffBus struct {
	mu    sync.Mutex
	queue []HandoffMessage
}

func NewHandoffBus() *HandoffBus {
	return &HandoffBus{queue: make([]HandoffMessage, 0, 128)}
}

func (h *HandoffBus) Push(m HandoffMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.queue = append(h.queue, m)
}

func (h *HandoffBus) Drain() []HandoffMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]HandoffMessage, len(h.queue))
	copy(out, h.queue)
	h.queue = h.queue[:0]
	return out
}
