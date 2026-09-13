package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/michaelxu2288/bullpen/internal/mesh"
	"github.com/spf13/cobra"
)

var swarmCmd = &cobra.Command{
	Use:   "crew",
	Short: "Boot a master coordinator + worker fleet and dispatch a goal",
	Long:  "Spins up a master node, registers N worker agents, splits the goal into work items, dispatches each to the least-loaded capable worker, and tails the internal gossip bus while results flow back.",
	RunE: func(cmd *cobra.Command, args []string) error {
		workers, _ := cmd.Flags().GetInt("workers")
		goal, _ := cmd.Flags().GetString("goal")
		if workers < 1 {
			workers = 3
		}

		master := mesh.NewMaster("master-0")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		for i := 1; i <= workers; i++ {
			node := mesh.Node{
				ID:           fmt.Sprintf("worker-%d", i),
				Provider:     "claude",
				Capabilities: []string{"code", "review"},
				MaxInFlight:  2,
			}
			w := mesh.NewWorker(node, master.Bus)
			w.Handle = func(_ context.Context, m mesh.Message) (string, error) {
				return "completed " + m.TaskID, nil
			}
			master.Register(node)
			go w.Run(ctx, 200*time.Millisecond, master.ID)
		}

		items := []mesh.WorkItem{
			{TaskID: "plan", Capability: "code", Prompt: goal + " :: decompose"},
			{TaskID: "impl", Capability: "code", Prompt: goal + " :: implement"},
			{TaskID: "rvw", Capability: "review", Prompt: goal + " :: review"},
		}
		fmt.Printf("master-0 dispatching %d items across %d workers\n", len(items), workers)
		for _, it := range items {
			node, err := master.Dispatch(it)
			if err != nil {
				fmt.Printf("  %s -> %v\n", it.TaskID, err)
				continue
			}
			fmt.Printf("  %s -> %s\n", it.TaskID, node)
		}

		time.Sleep(500 * time.Millisecond)
		fmt.Println("\n--- internal bus ---")
		for _, m := range master.Bus.Tail(20) {
			fmt.Printf("  [%s] %s -> %s %s\n", m.Kind, m.From, m.To, m.TaskID)
		}
		return nil
	},
}

func init() {
	swarmCmd.Flags().Int("workers", 3, "number of worker agents to register")
	swarmCmd.Flags().String("goal", "ship the feature", "goal to fan out across the crew")
	rootCmd.AddCommand(swarmCmd)
}
