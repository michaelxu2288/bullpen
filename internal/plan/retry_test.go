package plan

import "testing"

func TestRetryDelays(t *testing.T) {
	r := DefaultRetryPolicy()
	delays := r.Delays()
	if len(delays) != r.MaxAttempts {
		t.Fatalf("expected %d delays got %d", r.MaxAttempts, len(delays))
	}
	for i := 1; i < len(delays); i++ {
		if delays[i] < delays[i-1] {
			t.Fatalf("expected increasing backoff")
		}
	}
}
