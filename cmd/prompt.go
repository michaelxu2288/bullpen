package cmd

import (
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/crew"
	"github.com/spf13/cobra"
)

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Send a prompt to a running agent session",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		text, _ := cmd.Flags().GetString("text")

		mgr, err := crew.NewManager(repoPath)
		if err != nil {
			return err
		}
		if err := mgr.SendPrompt(name, text); err != nil {
			return err
		}
		fmt.Printf("prompt sent to %s\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(promptCmd)
	promptCmd.Flags().String("name", "", "Session name")
	promptCmd.Flags().String("text", "", "Prompt text to send")
	_ = promptCmd.MarkFlagRequired("name")
	_ = promptCmd.MarkFlagRequired("text")
}
