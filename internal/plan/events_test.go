package plan

import (
	"testing"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

func TestSubscriberReceivesPublishedEvents(t *testing.T) {
	bus := NewEventBus()
	stream, cancel := bus.Subscribe()
	defer cancel()

	if bus.SubscriberCount() != 1 {
		t.Fatalf("expected one subscriber, got %d", bus.SubscriberCount())
	}

	bus.Publish(domain.Event{ID: "e1", Type: domain.EventTaskCreated})

	select {
	case ev := <-stream:
		if ev.ID != "e1" {
			t.Fatalf("unexpected event %q", ev.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the event")
	}

	if len(bus.History()) != 1 {
		t.Fatal("history should still record the event")
	}
}

func TestSlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
	bus := NewEventBus()
	_, cancel := bus.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		// more than subscriberBuffer, with nobody draining
		for i := 0; i < subscriberBuffer*3; i++ {
			bus.Publish(domain.Event{ID: "e", Type: domain.EventToolInvoked})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked on a slow subscriber")
	}
}

func TestCancelRemovesTheSubscriber(t *testing.T) {
	bus := NewEventBus()
	stream, cancel := bus.Subscribe()
	cancel()

	if bus.SubscriberCount() != 0 {
		t.Fatalf("expected no subscribers after cancel, got %d", bus.SubscriberCount())
	}
	if _, open := <-stream; open {
		t.Fatal("expected the channel to be closed")
	}
	cancel() // must be idempotent
}
