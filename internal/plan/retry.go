package plan

import "time"

type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
	Multiplier  float64
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, Backoff: 400 * time.Millisecond, Multiplier: 1.8}
}

func (r RetryPolicy) Delays() []time.Duration {
	if r.MaxAttempts <= 0 {
		r.MaxAttempts = 1
	}
	if r.Backoff <= 0 {
		r.Backoff = 250 * time.Millisecond
	}
	if r.Multiplier <= 1 {
		r.Multiplier = 1.5
	}

	out := make([]time.Duration, 0, r.MaxAttempts)
	current := r.Backoff
	for i := 0; i < r.MaxAttempts; i++ {
		out = append(out, current)
		current = time.Duration(float64(current) * r.Multiplier)
	}
	return out
}
