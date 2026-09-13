package review

import "testing"

func TestNeedsHuman(t *testing.T) {
	g := HITLGate{Threshold: 0.9}
	if !g.NeedsHuman(0.7) {
		t.Fatalf("expected human review for low confidence")
	}
	if g.NeedsHuman(0.96) {
		t.Fatalf("did not expect human review for high confidence")
	}
}
