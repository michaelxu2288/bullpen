package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/michaelxu2288/bullpen/internal/mesh"
	"github.com/spf13/cobra"
)

var controlPlaneURL string

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "The webhook mesh: agents talking to and delegating to each other",
	Long: `Agents inside one bullpen process talk over the in-process bus. Agents in
separate processes — on this machine or another one — join the same fabric by
registering a webhook inbox.

Addressing is uniform either way: publish to an agent id and the bus decides
whether that means a channel send or a signed HTTP POST. Deliveries are
HMAC-SHA256 signed over "<timestamp>.<body>", retried with backoff, deduplicated
by delivery id on the receiving end, and a persistently unreachable agent is
quarantined so it stops costing every publish.`,
}

var agentsListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List the agents registered in the mesh",
	RunE: func(cmd *cobra.Command, args []string) error {
		var out struct {
			Self  string       `json:"self"`
			Peers []mesh.Peer `json:"peers"`
		}
		if err := meshRequest(http.MethodGet, "/v1/agents", nil, &out); err != nil {
			return err
		}

		fmt.Printf("coordinator: %s\n\n", out.Self)
		if len(out.Peers) == 0 {
			fmt.Println("no remote agents registered. join one with `bullpen agents register`.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "AGENT\tCAPABILITIES\tIN FLIGHT\tSTATE\tLAST SEEN\tURL")
		for _, p := range out.Peers {
			state := "ready"
			if p.Quarantined {
				state = "quarantined"
			} else if !p.Available() {
				state = "saturated"
			}
			capacity := fmt.Sprintf("%d/%d", p.InFlight, p.MaxInFlight)
			if p.MaxInFlight <= 0 {
				capacity = fmt.Sprintf("%d/-", p.InFlight)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				p.ID, strings.Join(p.Capabilities, ","), capacity, state,
				humanAgo(p.LastSeen), p.URL)
		}
		return w.Flush()
	},
}

var agentsRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Join an out-of-process agent to the mesh",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		url, _ := cmd.Flags().GetString("url")
		caps, _ := cmd.Flags().GetStringSlice("capability")
		maxInFlight, _ := cmd.Flags().GetInt("max-in-flight")
		secret, _ := cmd.Flags().GetString("secret")

		payload := map[string]any{
			"id": id, "url": url, "capabilities": caps, "max_in_flight": maxInFlight,
		}
		if secret != "" {
			payload["secret"] = secret
		}

		var out struct {
			Peer   mesh.Peer `json:"peer"`
			Secret string     `json:"secret"`
			Inbox  string     `json:"inbox"`
		}
		if err := meshRequest(http.MethodPost, "/v1/agents/register", payload, &out); err != nil {
			return err
		}

		fmt.Printf("registered %s\n", out.Peer.ID)
		fmt.Printf("  inbox:        %s\n", out.Peer.URL)
		fmt.Printf("  capabilities: %s\n", strings.Join(out.Peer.Capabilities, ", "))
		fmt.Printf("  secret:       %s\n", out.Secret)
		fmt.Println()
		fmt.Println("Store that secret: it signs every delivery to this agent and is shown once.")
		fmt.Println("Re-registering without --secret rotates it.")
		return nil
	},
}

var agentsDelegateCmd = &cobra.Command{
	Use:   "delegate",
	Short: "Hand a task from one agent to another",
	Long: `Delegate by name (--to) or by capability (--capability), in which case the
least-loaded capable agent wins, preferring an in-process worker over a remote
peer because a channel send beats a round trip.

The delegation chain travels with the task, so a cycle (a reviewer handing back
to the coder that handed it over) is refused rather than looping.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		capability, _ := cmd.Flags().GetString("capability")
		taskID, _ := cmd.Flags().GetString("task")
		prompt, _ := cmd.Flags().GetString("prompt")
		reason, _ := cmd.Flags().GetString("reason")
		chain, _ := cmd.Flags().GetStringSlice("chain")

		payload := mesh.Delegation{
			From: from, To: to, Capability: capability,
			TaskID: taskID, Prompt: prompt, Reason: reason, Chain: chain,
		}

		var result mesh.DelegationResult
		if err := meshRequest(http.MethodPost, "/v1/agents/delegate", payload, &result); err != nil {
			return err
		}

		where := "local bus"
		if result.Remote {
			where = "webhook"
		}
		fmt.Printf("%s -> %s  (%s, depth %d)\n", result.From, result.To, where, result.Depth)
		fmt.Printf("  task:  %s\n", result.TaskID)
		fmt.Printf("  chain: %s\n", strings.Join(append(result.Chain, result.To), " -> "))
		return nil
	},
}

var agentsSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Put a raw message on the mesh (broadcast with --to '*')",
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		kind, _ := cmd.Flags().GetString("kind")
		body, _ := cmd.Flags().GetString("body")
		taskID, _ := cmd.Flags().GetString("task")

		// A raw send is modelled as a delegation with an explicit target when it
		// is a handoff, and otherwise rides the mesh message endpoint.
		payload := mesh.Delegation{
			From: from, To: to, TaskID: taskID, Prompt: body,
			Reason: fmt.Sprintf("raw %s", kind),
		}
		if to == "*" {
			return fmt.Errorf("broadcast is not exposed over the CLI yet; use `bullpen agents delegate`")
		}

		var result mesh.DelegationResult
		if err := meshRequest(http.MethodPost, "/v1/agents/delegate", payload, &result); err != nil {
			return err
		}
		fmt.Printf("sent %s to %s\n", kind, result.To)
		return nil
	},
}

var agentsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Show recent traffic on the mesh wire",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")

		var messages []mesh.Message
		if err := meshRequest(http.MethodGet, "/v1/agents/messages", nil, &messages); err != nil {
			return err
		}
		if len(messages) > limit {
			messages = messages[len(messages)-limit:]
		}
		if len(messages) == 0 {
			fmt.Println("the wire is quiet.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TIME\tKIND\tFROM\tTO\tTASK\tBODY")
		for _, m := range messages {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				m.TS.Format("15:04:05"), m.Kind, m.From, m.To, m.TaskID, firstLine(m.Body, 48))
		}
		return w.Flush()
	},
}

func meshRequest(method, path string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, strings.TrimRight(controlPlaneURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("control plane unreachable at %s (is `bullpen server` running?): %w", controlPlaneURL, err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &errBody) == nil && errBody.Error != "" {
			return fmt.Errorf("%s", errBody.Error)
		}
		return fmt.Errorf("%s %s failed (%d): %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func firstLine(s string, max int) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

func humanAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}

func init() {
	rootCmd.AddCommand(agentsCmd)
	agentsCmd.PersistentFlags().StringVar(&controlPlaneURL, "control-plane", "http://127.0.0.1:7070", "bullpen server address")
	agentsCmd.AddCommand(agentsListCmd, agentsRegisterCmd, agentsDelegateCmd, agentsSendCmd, agentsTailCmd)

	agentsRegisterCmd.Flags().String("id", "", "Agent id, unique across the mesh")
	agentsRegisterCmd.Flags().String("url", "", "The agent's webhook inbox URL")
	agentsRegisterCmd.Flags().StringSlice("capability", nil, "Capability this agent advertises (repeatable)")
	agentsRegisterCmd.Flags().Int("max-in-flight", 2, "How many tasks it will hold at once (0 = unbounded)")
	agentsRegisterCmd.Flags().String("secret", "", "Pin a shared secret instead of minting one")
	_ = agentsRegisterCmd.MarkFlagRequired("id")
	_ = agentsRegisterCmd.MarkFlagRequired("url")

	agentsDelegateCmd.Flags().String("from", "", "Delegating agent")
	agentsDelegateCmd.Flags().String("to", "", "Explicit target agent")
	agentsDelegateCmd.Flags().String("capability", "", "Route to the least-loaded agent with this capability")
	agentsDelegateCmd.Flags().String("task", "", "Task id")
	agentsDelegateCmd.Flags().String("prompt", "", "What the delegate should do")
	agentsDelegateCmd.Flags().String("reason", "", "Why the sender is handing it over")
	agentsDelegateCmd.Flags().StringSlice("chain", nil, "Prior delegation hops, for cycle detection")
	_ = agentsDelegateCmd.MarkFlagRequired("from")
	_ = agentsDelegateCmd.MarkFlagRequired("task")

	agentsSendCmd.Flags().String("from", "", "Sending agent")
	agentsSendCmd.Flags().String("to", "", "Receiving agent")
	agentsSendCmd.Flags().String("kind", "handoff", "Message kind")
	agentsSendCmd.Flags().String("body", "", "Message body")
	agentsSendCmd.Flags().String("task", "", "Task id")
	_ = agentsSendCmd.MarkFlagRequired("from")
	_ = agentsSendCmd.MarkFlagRequired("to")

	agentsTailCmd.Flags().Int("limit", 30, "How many messages to show")
}
