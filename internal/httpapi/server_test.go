package httpapi

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/michaelxu2288/bullpen/internal/runners"
)

func TestShutdownDrainsAnOpenEventStream(t *testing.T) {
	h := NewHandlers(plan.NewEngine(runners.NewRegistry()))
	srv := NewServer("127.0.0.1:0", h)

	// bind manually so the test knows the port
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.httpServer.Serve(listener) }()

	url := "http://" + listener.Addr().String() + "/v1/stream"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	defer resp.Body.Close()

	// the stream is open and idle; shutdown must not wait out its deadline
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("shutdown blocked on the open stream for %s", elapsed)
	}
}
