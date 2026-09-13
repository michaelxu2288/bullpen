package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/michaelxu2288/bullpen/internal/runners"
	"github.com/michaelxu2288/bullpen/internal/mesh"
)

// fastSim makes every simulated task finish in tens of milliseconds.
func fastSim(failRate float64) mesh.SimOptions {
	return mesh.SimOptions{MinDuration: 30 * time.Millisecond, MaxDuration: 60 * time.Millisecond, FailRate: failRate, Seed: 42}
}

func newLive(t *testing.T, sim mesh.SimOptions) (*LiveRun, *BoardStore, context.CancelFunc) {
	t.Helper()
	engine := plan.NewEngine(runners.NewRegistry())
	board := NewBoardStore()
	live := NewLiveRun(NewMeshRouter("master-0"), board, engine.Events, sim)
	live.Tick = 40 * time.Millisecond
	live.Heartbeat = 15 * time.Millisecond
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return live, board, cancel
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLiveRunMovesCardsAcrossEveryLaneWithoutHumanInput(t *testing.T) {
	live, board, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 3)
	defer live.Stop()

	tasks := live.Submit("ship the checkout latency fix")
	if len(tasks) == 0 {
		t.Fatal("planner produced no tasks")
	}
	if board.Counts()[domain.TaskQueued] != len(tasks) {
		t.Fatal("submitted tasks should start in BACKLOG")
	}

	// cards must pass through RUNNING and REVIEW on their way to DONE
	sawRunning, sawReviewing := false, false
	waitFor(t, 5*time.Second, func() bool {
		counts := board.Counts()
		if counts[domain.TaskRunning] > 0 {
			sawRunning = true
		}
		if counts[domain.TaskReviewing] > 0 {
			sawReviewing = true
		}
		return counts[domain.TaskDone] == len(tasks)
	}, "every card to reach DONE")

	if !sawRunning || !sawReviewing {
		t.Fatalf("cards should be seen in RUNNING (%v) and REVIEW (%v) on the way", sawRunning, sawReviewing)
	}
	if board.Active() {
		t.Fatal("board should be idle once everything is done")
	}

	// every card was owned by a real worker, twice: once to code, once to review
	for _, task := range tasks {
		card, _ := board.Card(task.ID)
		if card.Owner == "" || card.Attempts != 2 {
			t.Fatalf("%s should show a reviewer owner and two pickups, got %+v", task.ID, card)
		}
		if card.Note != "approved" || card.Progress != -1 {
			t.Fatalf("%s should read approved with no meter once DONE, got %+v", task.ID, card)
		}
	}
}

func TestLiveRunWorksConcurrently(t *testing.T) {
	// slow enough that tasks overlap, fast enough for a test
	sim := mesh.SimOptions{MinDuration: 150 * time.Millisecond, MaxDuration: 200 * time.Millisecond, Seed: 5}
	live, board, _ := newLive(t, sim)
	live.Start(context.Background(), 3)
	defer live.Stop()

	live.Submit("parallelism")

	// three workers with two slots each: at some point several cards are in
	// flight at once, owned by different agents
	waitFor(t, 3*time.Second, func() bool {
		owners := map[string]bool{}
		inFlight := 0
		for _, lane := range board.Snapshot().Lanes {
			if lane.State != domain.TaskRunning {
				continue
			}
			for _, card := range lane.Cards {
				inFlight++
				owners[card.Owner] = true
			}
		}
		return inFlight >= 3 && len(owners) >= 2
	}, "three cards in RUNNING across at least two workers")
}

func TestLiveRunReportsRealProgress(t *testing.T) {
	sim := mesh.SimOptions{MinDuration: 200 * time.Millisecond, MaxDuration: 250 * time.Millisecond, Seed: 9}
	live, board, _ := newLive(t, sim)
	live.Start(context.Background(), 1)
	defer live.Stop()

	tasks := live.Submit("progress")
	id := tasks[0].ID

	var seen []float64
	waitFor(t, 3*time.Second, func() bool {
		card, _ := board.Card(id)
		if card.State == domain.TaskRunning && card.Progress >= 0 {
			if len(seen) == 0 || card.Progress != seen[len(seen)-1] {
				seen = append(seen, card.Progress)
			}
		}
		return len(seen) >= 3
	}, "at least three distinct progress values")

	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("progress went backwards: %v", seen)
		}
	}
	card, _ := board.Card(id)
	if card.Note == "" || card.Note == "picked up" {
		t.Fatalf("the card should carry the worker's stage note, got %q", card.Note)
	}
}

func TestLiveRunRequeuesAnEscalatedCard(t *testing.T) {
	live, board, _ := newLive(t, fastSim(1)) // every task escalates
	live.Start(context.Background(), 2)
	defer live.Stop()

	tasks := live.Submit("doomed")
	id := tasks[0].ID

	waitFor(t, 3*time.Second, func() bool {
		card, _ := board.Card(id)
		return card.Attempts >= 2
	}, "an escalated card to be picked up again")

	card, _ := board.Card(id)
	if card.State == domain.TaskDone {
		t.Fatal("a card that always fails must not reach DONE")
	}

	// the escalation reason should have been visible on the card at some point
	var sawReason bool
	for _, ev := range live.events.History() {
		if ev.Type == domain.EventHITLRequested && ev.Target == id {
			sawReason = true
		}
	}
	if !sawReason {
		t.Fatal("an escalation should land on the event timeline")
	}
}

func TestLiveRunNamespacesTasksPerGoal(t *testing.T) {
	live, board, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 1)
	defer live.Stop()

	a := live.Submit("first")
	b := live.Submit("second")
	if a[0].ID == b[0].ID {
		t.Fatalf("two goals must not collide on task ids: %s", a[0].ID)
	}
	if board.Snapshot().Total != len(a)+len(b) {
		t.Fatal("both goals should be on the board at once")
	}
}

func TestLiveRunStartIsIdempotentAndStopHalts(t *testing.T) {
	live, _, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 1)
	live.Start(context.Background(), 5) // no-op: must not spawn five more
	if len(live.Workers()) != 1 {
		t.Fatalf("second Start should be ignored, got %d workers", len(live.Workers()))
	}
	if !live.Running() {
		t.Fatal("expected running")
	}
	live.Stop()
	if live.Running() {
		t.Fatal("expected stopped")
	}
}

func TestHeartbeatsKeepWorkersAlive(t *testing.T) {
	live, _, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 2)
	defer live.Stop()

	// several detector ticks later, nobody has been reaped, because the
	// projection feeds heartbeats into the registry
	time.Sleep(8 * live.Tick)
	for _, node := range live.Workers() {
		if node.State != mesh.NodeAlive {
			t.Fatalf("%s should still be alive, got %s", node.ID, node.State)
		}
	}
}

func TestFeedSubmitsWhenTheBoardGoesIdle(t *testing.T) {
	live, board, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 2)
	defer live.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go live.Feed(ctx, []string{"one", "two"}, 30*time.Millisecond)

	// nothing submitted by hand; the feeder should notice the empty board,
	// submit "one", let it finish, and then submit "two"
	waitFor(t, 5*time.Second, func() bool { return board.Snapshot().Total >= 8 }, "two goals to be fed")

	seen := map[string]bool{}
	for _, lane := range board.Snapshot().Lanes {
		for _, card := range lane.Cards {
			seen[card.ID[:2]] = true
		}
	}
	if !seen["r1"] || !seen["r2"] {
		t.Fatalf("expected cards from two runs, got %v", seen)
	}
}

func TestKilledWorkerIsReapedAndItsCardRequeuedToASurvivor(t *testing.T) {
	// tasks long enough that a kill lands mid-flight, short enough that one
	// survivor can drain 4 cards x 2 phases on 2 slots inside the budget
	sim := mesh.SimOptions{MinDuration: 600 * time.Millisecond, MaxDuration: 600 * time.Millisecond, Seed: 11}
	live, board, _ := newLive(t, sim)
	live.Start(context.Background(), 2)
	defer live.Stop()

	tasks := live.Submit("resilience")

	// wait until someone is holding a card
	var victim, cardID string
	waitFor(t, 3*time.Second, func() bool {
		for _, lane := range board.Snapshot().Lanes {
			if lane.State != domain.TaskRunning {
				continue
			}
			for _, card := range lane.Cards {
				if card.Owner != "" {
					victim, cardID = card.Owner, card.ID
					return true
				}
			}
		}
		return false
	}, "a worker to pick up a card")

	if err := live.KillWorker(victim); err != nil {
		t.Fatalf("kill failed: %v", err)
	}
	if err := live.KillWorker(victim); err == nil {
		t.Fatal("killing twice should fail")
	}

	// SWIM: alive -> suspect -> dead, on the detector's clock, not ours
	waitFor(t, 3*time.Second, func() bool {
		for _, node := range live.Workers() {
			if node.ID == victim && node.State == mesh.NodeDead {
				return true
			}
		}
		return false
	}, "the detector to declare "+victim+" dead")

	waitFor(t, 3*time.Second, func() bool {
		card, _ := board.Card(cardID)
		return card.Requeues >= 1 && card.State == domain.TaskQueued
	}, "the card to be requeued")
	if card, _ := board.Card(cardID); !strings.Contains(card.Note, "worker died") {
		t.Fatalf("the card should say why it bounced, got %q", card.Note)
	}

	// and then picked up again by the survivor, and finished
	waitFor(t, 12*time.Second, func() bool {
		card, _ := board.Card(cardID)
		return card.State == domain.TaskDone
	}, "the survivor to finish the requeued card")

	card, _ := board.Card(cardID)
	if card.Owner == victim {
		t.Fatalf("the dead worker must not end up owning the card, got %+v", card)
	}
	if card.Requeues < 1 {
		t.Fatalf("the bounce should be counted, got %+v", card)
	}
	_ = tasks
}

func TestRevivedWorkerComesBackAlive(t *testing.T) {
	live, _, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 2)
	defer live.Stop()

	if err := live.KillWorker("worker-1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		for _, node := range live.Workers() {
			if node.ID == "worker-1" && node.State == mesh.NodeDead {
				return true
			}
		}
		return false
	}, "worker-1 to be declared dead")

	if _, err := live.ReviveWorker("worker-1"); err != nil {
		t.Fatalf("revive failed: %v", err)
	}
	if _, err := live.ReviveWorker("worker-1"); err == nil {
		t.Fatal("reviving a live worker should fail")
	}

	waitFor(t, 2*time.Second, func() bool {
		for _, node := range live.Workers() {
			if node.ID == "worker-1" && node.State == mesh.NodeAlive {
				return true
			}
		}
		return false
	}, "worker-1 to heartbeat back to alive")
	if len(live.Workers()) != 2 {
		t.Fatalf("revive must reuse the id, not add a node; got %d workers", len(live.Workers()))
	}
}

func TestChaosNeverTakesTheFleetBelowTwo(t *testing.T) {
	live, _, _ := newLive(t, fastSim(0))
	live.Start(context.Background(), 2)
	defer live.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go live.Chaos(ctx, 30*time.Millisecond, 60*time.Millisecond)

	time.Sleep(300 * time.Millisecond)
	// with only two workers, chaos may kill one at a time but must revive
	// before killing again: at no point are both dead
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		live.mu.Lock()
		running := len(live.workers)
		live.mu.Unlock()
		if running == 0 {
			t.Fatal("chaos killed the whole fleet")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCardNarratesItsOwnerGoingSuspect(t *testing.T) {
	// a slow detector so the suspect window is wide enough to observe
	sim := mesh.SimOptions{MinDuration: 5 * time.Second, MaxDuration: 5 * time.Second, Seed: 2}
	live, board, _ := newLive(t, sim)
	live.Tick = 200 * time.Millisecond // suspect at 600ms, dead at 1.2s
	live.Heartbeat = 30 * time.Millisecond
	live.Start(context.Background(), 1)
	defer live.Stop()

	tasks := live.Submit("narration")
	id := tasks[0].ID
	deadline := time.Now().Add(3 * time.Second)
	for {
		card, _ := board.Card(id)
		if card.State == domain.TaskRunning {
			break
		}
		if time.Now().After(deadline) {
			live.mu.Lock()
			pending := len(live.pending)
			live.mu.Unlock()
			t.Fatalf("no pickup: card=%+v workers=%+v pending=%d", card, live.Workers(), pending)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := live.KillWorker("worker-1"); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 2*time.Second, func() bool {
		card, _ := board.Card(id)
		return strings.Contains(card.Note, "stopped responding")
	}, "the card to report its owner stopped responding")

	waitFor(t, 3*time.Second, func() bool {
		card, _ := board.Card(id)
		return card.State == domain.TaskQueued && strings.Contains(card.Note, "worker died")
	}, "the card to be requeued once the owner is declared dead")
}

func TestDispatchHonoursPriority(t *testing.T) {
	// one worker, one slot: the first card dispatched is the only one running
	sim := mesh.SimOptions{MinDuration: time.Second, MaxDuration: time.Second, Seed: 1}
	live, board, _ := newLive(t, sim)
	live.Start(context.Background(), 0)
	defer live.Stop()

	// spawn a single-slot worker by hand
	node := mesh.Node{ID: "solo", Capabilities: []string{"code", "review"}, MaxInFlight: 1}
	w := mesh.NewWorker(node, live.router.Bus)
	w.HandleWithProgress = mesh.SimulatedHandler(sim)
	live.master.Register(node)
	go w.Run(live.ctx, live.Heartbeat, live.master.ID)

	for i := 0; i < 5; i++ {
		tasks := live.Submit("priority")
		_ = tasks
		waitFor(t, 2*time.Second, func() bool {
			return board.Counts()[domain.TaskRunning] == 1
		}, "one card running")

		var running BoardCard
		for _, lane := range board.Snapshot().Lanes {
			if lane.State == domain.TaskRunning && len(lane.Cards) > 0 {
				running = lane.Cards[0]
			}
		}
		if running.Priority != 1 {
			t.Fatalf("run %d: the p1 card should be dispatched first, got %s (p%d)", i, running.ID, running.Priority)
		}
		live.Stop()
		live, board, _ = newLive(t, sim)
		live.Start(context.Background(), 0)
		w = mesh.NewWorker(node, live.router.Bus)
		w.HandleWithProgress = mesh.SimulatedHandler(sim)
		live.master.Register(node)
		go w.Run(live.ctx, live.Heartbeat, live.master.ID)
	}
}
