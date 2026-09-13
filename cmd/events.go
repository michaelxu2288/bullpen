package cmd

import (
	"context"
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/spf13/cobra"
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Run a quick orchestration and print internal event stream",
	RunE: func(cmd *cobra.Command, args []string) error {
		engine := newEngine()
		_, err := engine.Run(context.Background(), plan.RunInput{Goal: "generate event stream", TraceID: "trace-events-1"})
		if err != nil {
			return err
		}
		for _, ev := range engine.Events.History() {
			fmt.Printf("%s | %s -> %s\n", ev.Type, ev.Actor, ev.Target)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(eventsCmd)
}
