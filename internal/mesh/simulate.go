package mesh

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strings"
	"time"
)

// SimOptions shapes a simulated crew. Durations are per task; the handler
// picks a point in the range so parallel cards visibly finish at different
// times instead of in lockstep.
type SimOptions struct {
	MinDuration time.Duration
	MaxDuration time.Duration
	// FailRate is the probability, 0..1, that a task escalates instead of
	// completing. Failures are what make the requeue path visible.
	FailRate float64
	// Seed makes a run reproducible. Zero seeds from the clock.
	Seed int64
}

func DefaultSimOptions() SimOptions {
	return SimOptions{
		MinDuration: 4 * time.Second,
		MaxDuration: 11 * time.Second,
		FailRate:    0.08,
	}
}

// Stage notes are per capability, so a coder card and a reviewer card narrate
// different work while they crawl across the board.
var simStages = map[string][]string{
	"code": {
		"retrieving context",
		"reading the surrounding code",
		"drafting the change",
		"writing tests",
		"running the suite",
		"tidying the diff",
	},
	"review": {
		"retrieving the incident thread",
		"reading the diff",
		"checking blast radius",
		"tracing the failure mode",
		"writing review notes",
	},
	"plan": {
		"retrieving prior plans",
		"decomposing the goal",
		"sizing the pieces",
		"ordering by dependency",
	},
}

// SimulatedHandler returns a HandleWithProgress that behaves like an agent
// working: it steps through capability-appropriate stages, reports progress at
// each, takes a plausible amount of wall time, and occasionally escalates.
//
// It is the stand-in for a provider adapter driving a real coding agent, and
// it exists so the board can be watched without a Claude or Codex CLI in the
// loop. Everything it emits travels over the same bus messages a real worker
// would send.
func SimulatedHandler(opts SimOptions) func(context.Context, Message, ProgressFunc) (string, error) {
	if opts.MinDuration <= 0 {
		opts.MinDuration = time.Second
	}
	if opts.MaxDuration < opts.MinDuration {
		opts.MaxDuration = opts.MinDuration
	}

	return func(ctx context.Context, m Message, report ProgressFunc) (string, error) {
		// Seed per task so a given card behaves the same way every run, which
		// keeps demos and tests reproducible without making the fleet uniform.
		rng := rand.New(rand.NewSource(taskSeed(opts.Seed, m.TaskID, m.To)))

		capability := m.Headers["capability"]
		stages, ok := simStages[capability]
		if !ok {
			stages = simStages["code"]
		}

		span := opts.MaxDuration - opts.MinDuration
		total := opts.MinDuration
		if span > 0 {
			total += time.Duration(rng.Int63n(int64(span)))
		}
		perStage := total / time.Duration(len(stages))

		failAt := -1
		if opts.FailRate > 0 && rng.Float64() < opts.FailRate {
			// fail somewhere past the first stage, so the card is visibly
			// in flight when it escalates
			failAt = 1 + rng.Intn(len(stages)-1)
		}

		for i, stage := range stages {
			report(float64(i)/float64(len(stages)), stage)

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(jitter(rng, perStage)):
			}

			if i == failAt {
				return "", fmt.Errorf("%s: %s", stage, failureFor(rng, capability))
			}
		}
		report(1, "done")

		return fmt.Sprintf("%s complete: %s", capability, strings.TrimSpace(m.Body)), nil
	}
}

var failures = map[string][]string{
	"code": {
		"tests failed on the retry path",
		"merge conflict in internal/plan",
		"the change needs a schema migration first",
	},
	"review": {
		"blast radius exceeds the freeze-window policy",
		"missing rollback plan",
	},
	"plan": {
		"goal is underspecified; need the incident id",
	},
}

func failureFor(rng *rand.Rand, capability string) string {
	options, ok := failures[capability]
	if !ok {
		options = failures["code"]
	}
	return options[rng.Intn(len(options))]
}

func jitter(rng *rand.Rand, d time.Duration) time.Duration {
	// +/- 30 percent, so parallel stages do not tick in unison
	factor := 0.7 + rng.Float64()*0.6
	return time.Duration(float64(d) * factor)
}

func taskSeed(base int64, parts ...string) int64 {
	if base == 0 {
		base = time.Now().UnixNano()
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%d", base)
	for _, p := range parts {
		h.Write([]byte(p))
	}
	return int64(h.Sum64())
}
