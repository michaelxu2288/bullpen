package cmd

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/michaelxu2288/bullpen/internal/httpapi"
	"github.com/spf13/cobra"
)

var fleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "The live worker fleet: list, kill, revive",
	Long: `The fleet is the set of worker agents a running ` + "`bullpen server`" + ` dispatches
to. Killing one is the way to watch the failure path: its heartbeats stop, the
SWIM detector moves it alive -> suspect -> dead, and the cards it was holding
bounce back to BACKLOG and get picked up by a survivor. Reviving brings it back
under the same id with a new incarnation.`,
}

var fleetListCmd = &cobra.Command{
	Use:   "ls",
	Short: "Show every worker with its liveness and what it is holding",
	RunE: func(cmd *cobra.Command, args []string) error {
		var rows []httpapi.WorkerRow
		if err := meshRequest(http.MethodGet, "/v1/workers", nil, &rows); err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Println("no live fleet. start the server with --live (the default).")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "WORKER\tPROVIDER\tSTATE\tIN FLIGHT\tHOLDING\tHEARTBEAT")
		for _, r := range rows {
			holding := "-"
			if len(r.Holding) > 0 {
				holding = strings.Join(r.Holding, ",")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d/%d\t%s\t%s\n",
				r.ID, r.Provider, r.State, r.InFlight, r.MaxInFlight, holding, humanAgo(r.LastHeartbeat))
		}
		return w.Flush()
	},
}

var fleetKillCmd = &cobra.Command{
	Use:   "kill <worker>",
	Short: "Stop a worker dead and watch the detector notice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var out struct {
			Killed string `json:"killed"`
			Note   string `json:"note"`
		}
		if err := meshRequest(http.MethodPost, "/v1/workers/kill", map[string]string{"id": args[0]}, &out); err != nil {
			return err
		}
		fmt.Printf("killed %s\n  %s\n", out.Killed, out.Note)
		return nil
	},
}

var fleetReviveCmd = &cobra.Command{
	Use:   "revive <worker>",
	Short: "Bring a killed worker back under the same id",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var out struct {
			Revived     string `json:"revived"`
			Incarnation uint64 `json:"incarnation"`
		}
		if err := meshRequest(http.MethodPost, "/v1/workers/revive", map[string]string{"id": args[0]}, &out); err != nil {
			return err
		}
		fmt.Printf("revived %s (incarnation %d)\n", out.Revived, out.Incarnation)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(fleetCmd)
	fleetCmd.PersistentFlags().StringVar(&controlPlaneURL, "control-plane", "http://127.0.0.1:7070", "bullpen server address")
	fleetCmd.AddCommand(fleetListCmd, fleetKillCmd, fleetReviveCmd)
}
