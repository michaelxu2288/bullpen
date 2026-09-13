package cmd

import (
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/crew"
	"github.com/michaelxu2288/bullpen/internal/hooks"
	"github.com/spf13/cobra"
)

var handoffCmd = &cobra.Command{
	Use:   "handoff",
	Short: "Pipe recent context from one agent session into another",
	RunE: func(cmd *cobra.Command, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		target, _ := cmd.Flags().GetString("target")
		modeStr, _ := cmd.Flags().GetString("mode")
		maxLines, _ := cmd.Flags().GetInt("max-lines")

		mgr, err := crew.NewManager(repoPath)
		if err != nil {
			return err
		}

		mode := hooks.Mode(modeStr)
		if err := mgr.PipePrompt(source, target, mode, maxLines); err != nil {
			return err
		}

		fmt.Printf("handoff complete: %s -> %s (%s)\n", source, target, mode)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(handoffCmd)
	handoffCmd.Flags().String("source", "", "Source session name")
	handoffCmd.Flags().String("target", "", "Target session name")
	handoffCmd.Flags().String("mode", string(hooks.ModeSummary), "Hook mode: summary|bugfix|review")
	handoffCmd.Flags().Int("max-lines", 120, "Number of latest source lines to include in handoff")
	_ = handoffCmd.MarkFlagRequired("source")
	_ = handoffCmd.MarkFlagRequired("target")
}
