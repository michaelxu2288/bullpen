package cmd

import (
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/config"
	"github.com/spf13/cobra"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Initialize default profiles, orchestration settings, and tool-ready config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.DefaultConfig()
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Println("initialized config with default Claude/Codex/Aider/Gemini profiles")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(bootstrapCmd)
}
