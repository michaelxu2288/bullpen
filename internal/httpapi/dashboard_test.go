package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/michaelxu2288/bullpen/internal/runners"
	"github.com/michaelxu2288/bullpen/internal/mesh"
)

func testHandlers(t *testing.T) (*Handlers, *http.ServeMux) {
	t.Helper()
	engine := plan.NewEngine(runners.NewRegistry())
	h := NewHandlers(engine)
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func seedBoard(h *Handlers) {
	now := time.Now()
	h.Board.Replace([]domain.Task{
		{ID: "t-01", Title: "plan the fix", State: domain.TaskQueued, Priority: 1, OwnerSession: "planner-claude", UpdatedAt: now},
		{ID: "t-02", Title: "write the patch", State: domain.TaskRunning, Priority: 2, OwnerSession: "coder-codex", UpdatedAt: now},
		{ID: "t-03", Title: "review blast radius", State: domain.TaskReviewing, Priority: 1, OwnerSession: "reviewer-claude", UpdatedAt: now},
	})
}

func TestBoardGroupsCardsIntoLanesInOrder(t *testing.T) {
	h, mux := testHandlers(t)
	seedBoard(h)

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/board", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", res.Code)
	}

	var board Board
	if err := json.Unmarshal(res.Body.Bytes(), &board); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(board.Lanes) != 4 {
		t.Fatalf("expected 4 lanes, got %d", len(board.Lanes))
	}
	if board.Lanes[0].Title != "BACKLOG" || board.Lanes[3].Title != "DONE" {
		t.Fatalf("unexpected lane order: %s..%s", board.Lanes[0].Title, board.Lanes[3].Title)
	}
	if board.Total != 3 {
		t.Fatalf("expected 3 cards, got %d", board.Total)
	}
	// progress is real data now: nothing has reported, so nothing shows any
	for _, lane := range board.Lanes {
		for _, card := range lane.Cards {
			if card.Progress != -1 {
				t.Fatalf("%s should have no progress before an agent reports, got %v", card.ID, card.Progress)
			}
		}
	}
}

func TestProgressComesFromWorkerReports(t *testing.T) {
	h, _ := testHandlers(t)
	seedBoard(h)

	if err := h.Board.Assign("t-01", "coder-1", domain.TaskRunning); err != nil {
		t.Fatal(err)
	}
	h.Board.SetProgress("t-01", 0.42, "writing tests")

	card, _ := h.Board.Card("t-01")
	if card.Progress != 0.42 || card.Note != "writing tests" || card.Owner != "coder-1" {
		t.Fatalf("card should reflect the report, got %+v", card)
	}
	if card.State != domain.TaskRunning || card.Attempts != 1 {
		t.Fatalf("assign should move the lane and count the attempt, got %+v", card)
	}

	if err := h.Board.Requeue("t-01", "worker died"); err != nil {
		t.Fatal(err)
	}
	card, _ = h.Board.Card("t-01")
	if card.State != domain.TaskQueued || card.Owner != "" || card.Progress != -1 || card.Note != "worker died" {
		t.Fatalf("requeue should reset the card and keep the reason, got %+v", card)
	}

	// a second pickup is a second attempt, and the bounce is counted separately
	_ = h.Board.Assign("t-01", "coder-2", domain.TaskRunning)
	card, _ = h.Board.Card("t-01")
	if card.Attempts != 2 || card.Requeues != 1 {
		t.Fatalf("expected attempts=2 requeues=1, got %+v", card)
	}
}

func TestBoardChangeNotifierCoalesces(t *testing.T) {
	h, _ := testHandlers(t)
	wake, stop := h.Board.Changed.Subscribe()
	defer stop()

	seedBoard(h)
	h.Board.SetProgress("t-02", 0.1, "")
	h.Board.SetProgress("t-02", 0.2, "")

	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("a mutation should wake the subscriber")
	}
	select {
	case <-wake:
		t.Fatal("three mutations should coalesce into one pending wake-up")
	default:
	}
}

func TestAdvanceMovesRightAndStopsAtDone(t *testing.T) {
	h, mux := testHandlers(t)
	seedBoard(h)

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/board/advance", strings.NewReader(body))
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		return res
	}

	res := post(`{"id":"t-03"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("advance failed: %d %s", res.Code, res.Body.String())
	}
	var card BoardCard
	_ = json.Unmarshal(res.Body.Bytes(), &card)
	if card.State != domain.TaskDone {
		t.Fatalf("expected review -> done, got %s", card.State)
	}

	if again := post(`{"id":"t-03"}`); again.Code != http.StatusBadRequest {
		t.Fatalf("advancing past DONE should fail, got %d", again.Code)
	}
	if missing := post(`{"id":"nope"}`); missing.Code != http.StatusBadRequest {
		t.Fatalf("unknown card should fail, got %d", missing.Code)
	}
	if noID := post(`{}`); noID.Code != http.StatusBadRequest {
		t.Fatalf("missing id should fail, got %d", noID.Code)
	}
}

func TestAdvanceAcceptsAnExplicitLaneAndPublishesAnEvent(t *testing.T) {
	h, mux := testHandlers(t)
	seedBoard(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/board/advance", strings.NewReader(`{"id":"t-01","state":"reviewing"}`))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", res.Code, res.Body.String())
	}

	events := h.Engine.Events.History()
	if len(events) != 1 || events[len(events)-1].Type != domain.EventTaskStateChanged {
		t.Fatalf("expected a state-changed event, got %+v", events)
	}

	bad := httptest.NewRequest(http.MethodPost, "/v1/board/advance", strings.NewReader(`{"id":"t-01","state":"nonsense"}`))
	badRes := httptest.NewRecorder()
	mux.ServeHTTP(badRes, bad)
	if badRes.Code != http.StatusBadRequest {
		t.Fatalf("unknown lane should fail, got %d", badRes.Code)
	}
}

func TestAdvanceRejectsGet(t *testing.T) {
	_, mux := testHandlers(t)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/board/advance", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", res.Code)
	}
}

func TestSummaryCountsLanesAndReportsNoPlane(t *testing.T) {
	h, mux := testHandlers(t)
	seedBoard(h)

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/summary", nil))

	var summary Summary
	if err := json.Unmarshal(res.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if summary.TotalTasks != 3 || summary.Tasks["running"] != 1 {
		t.Fatalf("unexpected task counts %+v", summary.Tasks)
	}
	if summary.Uptime == "" {
		t.Fatal("expected an uptime string")
	}
}

func TestStreamReplaysHistoryThenLiveEvents(t *testing.T) {
	h, mux := testHandlers(t)
	h.Engine.Events.Publish(domain.Event{ID: "old", Type: domain.EventTaskCreated, CreatedAt: time.Now()})

	server := httptest.NewServer(mux)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("unexpected content type %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	if line := readFrame(t, reader, "event"); !strings.Contains(line, `"old"`) {
		t.Fatalf("expected the replayed event first, got %q", line)
	}
	// the initial board snapshot rides along on open
	if line := readFrame(t, reader, "board"); !strings.Contains(line, `"lanes"`) {
		t.Fatalf("expected a board snapshot on open, got %q", line)
	}

	// wait for the subscription to land, then publish a live one
	deadline := time.Now().Add(2 * time.Second)
	for h.Engine.Events.SubscriberCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	h.Engine.Events.Publish(domain.Event{ID: "live", Type: domain.EventToolInvoked, CreatedAt: time.Now()})

	if line := readFrame(t, reader, "event"); !strings.Contains(line, `"live"`) {
		t.Fatalf("expected the live event, got %q", line)
	}

	// a board mutation pushes a fresh snapshot, throttled, without a poll
	h.Board.Add([]domain.Task{{ID: "pushed", Title: "pushed card", State: domain.TaskQueued}})
	if line := readFrame(t, reader, "board"); !strings.Contains(line, `"pushed"`) {
		t.Fatalf("expected the mutated board to be pushed, got %q", line)
	}
}

// readFrame returns the data line of the next SSE frame with the given name,
// skipping frames of other kinds.
func readFrame(t *testing.T, reader *bufio.Reader, want string) string {
	t.Helper()
	current := ""
	for i := 0; i < 200; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("stream read failed: %v", err)
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			current = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: ") && current == want:
			return line
		}
	}
	t.Fatalf("no %q frame in the stream", want)
	return ""
}

func TestSessionsAnswersEvenWithNoStore(t *testing.T) {
	_, mux := testHandlers(t)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/sessions", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	var rows []AgentRow
	if err := json.Unmarshal(res.Body.Bytes(), &rows); err != nil {
		t.Fatalf("expected a json array, got %s", res.Body.String())
	}
}

func TestUIIsOnlyMountedWhenEnabled(t *testing.T) {
	engine := plan.NewEngine(runners.NewRegistry())
	mux := http.NewServeMux()
	NewHandlers(engine).WithUI(true).Register(mux)

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	// unbuilt bundle answers 503 with instructions rather than 404
	if res.Code != http.StatusServiceUnavailable && res.Code != http.StatusOK {
		t.Fatalf("unexpected status for the ui root: %d", res.Code)
	}
}

func TestAnnotateIfHeldRefusesStaleWrites(t *testing.T) {
	h, _ := testHandlers(t)
	seedBoard(h)
	_ = h.Board.Assign("t-01", "worker-1", domain.TaskRunning)

	if !h.Board.AnnotateIfHeld("t-01", "worker-1", "worker-1 stopped responding") {
		t.Fatal("a held card should accept the annotation")
	}
	if h.Board.AnnotateIfHeld("t-01", "worker-1", "worker-1 stopped responding") {
		t.Fatal("an identical note should be a no-op, not a change event")
	}

	_ = h.Board.Requeue("t-01", "worker died; requeued")
	if h.Board.AnnotateIfHeld("t-01", "worker-1", "worker-1 is dead") {
		t.Fatal("a requeued card is no longer held; the stale note must be refused")
	}
	card, _ := h.Board.Card("t-01")
	if card.Note != "worker died; requeued" {
		t.Fatalf("the requeue reason should survive, got %q", card.Note)
	}

	if h.Board.AnnotateIfHeld("t-02", "somebody-else", "x") {
		t.Fatal("a card held by another owner must not be annotated")
	}
}

func TestKillAndReviveEndpoints(t *testing.T) {
	h, mux := testHandlers(t)

	// no live crew attached: honest 503, not a nil deref
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/workers/kill", strings.NewReader(`{"id":"worker-1"}`)))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without a live crew, got %d", res.Code)
	}

	live := NewLiveRun(h.Mesh, h.Board, h.Engine.Events, mesh.SimOptions{MinDuration: time.Second, MaxDuration: time.Second})
	live.Tick = 40 * time.Millisecond
	live.Heartbeat = 15 * time.Millisecond
	live.Start(context.Background(), 2)
	defer live.Stop()
	h.WithLive(live)

	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/workers/kill", strings.NewReader(`{"id":"worker-2"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("kill failed: %d %s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/workers/kill", strings.NewReader(`{"id":"ghost"}`)))
	if res.Code != http.StatusNotFound {
		t.Fatalf("killing an unknown worker should 404, got %d", res.Code)
	}

	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/workers/revive", strings.NewReader(`{"id":"worker-2"}`)))
	if res.Code != http.StatusOK {
		t.Fatalf("revive failed: %d %s", res.Code, res.Body.String())
	}
	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/workers/revive", strings.NewReader(`{"id":"worker-2"}`)))
	if res.Code != http.StatusConflict {
		t.Fatalf("reviving a live worker should 409, got %d", res.Code)
	}

	res = httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodPost, "/v1/workers/kill", strings.NewReader(`{}`)))
	if res.Code != http.StatusBadRequest {
		t.Fatalf("missing id should 400, got %d", res.Code)
	}
}
