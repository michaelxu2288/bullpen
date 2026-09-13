package cmd

import (
	"fmt"
	"time"

	"github.com/michaelxu2288/bullpen/internal/telemetry"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a manager-facing orchestration report snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		metrics := telemetry.NewMetrics()
		metrics.Add("sessions.active", 4)
		metrics.Add("tasks.running", 7)
		metrics.Add("handoffs.total", 13)
		metrics.Add("rag.queries", 9)
		metrics.Add("mcp.calls", 5)

		audit := &telemetry.Auditor{}
		audit.Record("planner-claude", "plan", "task-1", "ok")
		audit.Record("coder-codex", "implement", "task-1", "ok")
		audit.Record("reviewer-claude", "review", "task-1", "escalated")

		sink := telemetry.TraceSink{Dir: ".cc-agent-traces"}
		_ = sink.Write(telemetry.TraceSpan{
			TraceID:   "trace-report-001",
			SpanID:    "span-1",
			Name:      "manager.report",
			Attrs:     map[string]string{"source": "cli", "env": "mvp"},
			StartedAt: time.Now().Add(-time.Second),
			EndedAt:   time.Now(),
		})

		fmt.Println("=== METRICS ===")
		for _, c := range metrics.Snapshot() {
			fmt.Printf("%s=%d\n", c.Name, c.Value)
		}
		fmt.Println("\n=== AUDIT ===")
		fmt.Print(audit.Render())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(reportCmd)
}
