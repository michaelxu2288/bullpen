package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michaelxu2288/bullpen/internal/httpapi"
	"github.com/michaelxu2288/bullpen/internal/mesh"
	"github.com/michaelxu2288/bullpen/internal/web"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the HTTP control plane, the web dashboard, and a live crew",
	Long: `Serves the JSON API and the embedded dashboard on one port.

With --live (the default) it also boots a fleet of simulated worker agents and
projects everything they do onto the board: the master dispatches, workers
execute and report progress over the bus, coders hand finished cards to
reviewers, and cards that escalate go back to BACKLOG for someone else. Nothing
on the board is placed by hand.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		// Replit hands the bound port in $PORT and routes the webview at it, so
		// the flag default gives way to the environment when one is set.
		if port := os.Getenv("PORT"); port != "" && !cmd.Flags().Changed("addr") {
			addr = ":" + port
		}
		serveUI, _ := cmd.Flags().GetBool("ui")
		seedGoal, _ := cmd.Flags().GetString("seed")
		live, _ := cmd.Flags().GetBool("live")
		workers, _ := cmd.Flags().GetInt("workers")
		minDur, _ := cmd.Flags().GetDuration("task-min")
		maxDur, _ := cmd.Flags().GetDuration("task-max")
		failRate, _ := cmd.Flags().GetFloat64("fail-rate")
		feed, _ := cmd.Flags().GetBool("feed")
		chaos, _ := cmd.Flags().GetBool("chaos")
		chaosEvery, _ := cmd.Flags().GetDuration("chaos-every")
		chaosDown, _ := cmd.Flags().GetDuration("chaos-down")

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		engine := newEngine()
		h := httpapi.NewHandlers(engine).WithUI(serveUI)

		if live {
			run := httpapi.NewLiveRun(h.Mesh, h.Board, engine.Events, mesh.SimOptions{
				MinDuration: minDur,
				MaxDuration: maxDur,
				FailRate:    failRate,
			})
			run.Start(ctx, workers)
			defer run.Stop()
			h = h.WithLive(run)
			fmt.Printf("live crew: %d workers, tasks take %s-%s, %.0f%% escalate\n",
				workers, minDur, maxDur, failRate*100)

			if seedGoal != "" {
				tasks := run.Submit(seedGoal)
				fmt.Printf("submitted %q as %d tasks\n", seedGoal, len(tasks))
			}
			if feed {
				go run.Feed(ctx, httpapi.DemoGoals, 4*time.Second)
				fmt.Println("feeder on: a new goal lands whenever the board goes idle")
			}
			if chaos {
				go run.Chaos(ctx, chaosEvery, chaosDown)
				fmt.Printf("chaos on: a random worker dies every %s and comes back %s later\n", chaosEvery, chaosDown)
			}
		} else if seedGoal != "" {
			// no crew: fall back to a static plan so the board is not empty
			result, err := engine.Run(ctx, orchestrationInput(seedGoal))
			if err != nil {
				return fmt.Errorf("failed to seed the board: %w", err)
			}
			h.Board.ReplaceWithOwners(result.Tasks, result.SessionByTask)
			fmt.Printf("board seeded statically with %d tasks\n", len(result.Tasks))
		}

		srv := httpapi.NewServer(addr, h)
		if serveUI {
			if web.Built() {
				fmt.Printf("dashboard on http://localhost%s\n", addr)
			} else {
				fmt.Println("dashboard bundle not built; run `make build-web` (the JSON API still works)")
			}
		}
		fmt.Printf("api listening on %s\n", addr)

		errCh := make(chan error, 1)
		go func() { errCh <- srv.Start() }()

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			fmt.Println("\nshutting down")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().String("addr", ":7070", "Bind address")
	serverCmd.Flags().Bool("ui", true, "Serve the embedded React dashboard at /")
	serverCmd.Flags().String("seed", "ship the checkout latency fix", "Goal to submit on start (empty to start blank)")
	serverCmd.Flags().Bool("live", true, "Boot simulated workers and run the board live")
	serverCmd.Flags().Int("workers", 3, "Simulated workers to boot with --live")
	serverCmd.Flags().Duration("task-min", 4*time.Second, "Shortest a simulated task takes")
	serverCmd.Flags().Duration("task-max", 11*time.Second, "Longest a simulated task takes")
	serverCmd.Flags().Float64("fail-rate", 0.08, "Fraction of simulated tasks that escalate and requeue")
	serverCmd.Flags().Bool("feed", true, "Submit a new goal whenever the board goes idle")
	serverCmd.Flags().Bool("chaos", false, "Periodically kill a random worker and revive it later")
	serverCmd.Flags().Duration("chaos-every", 25*time.Second, "How often chaos strikes")
	serverCmd.Flags().Duration("chaos-down", 8*time.Second, "How long a killed worker stays down")
}
