package telemetry

import "sync"

type Counter struct {
	Name  string
	Value int64
}

type Metrics struct {
	mu       sync.Mutex
	counters map[string]int64
}

func NewMetrics() *Metrics {
	return &Metrics{counters: map[string]int64{}}
}

func (m *Metrics) Inc(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name]++
}

func (m *Metrics) Add(name string, v int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += v
}

func (m *Metrics) Snapshot() []Counter {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Counter, 0, len(m.counters))
	for k, v := range m.counters {
		out = append(out, Counter{Name: k, Value: v})
	}
	return out
}
