// beads-cost is a standalone sidecar binary for tracking LLM session costs.
// It stores data in .beads/cost.db (SQLite), independently of the main
// beads/dolt database.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads-cost/internal/cost"
)

var (
	beadsDir   string
	jsonOutput bool

	// Set via ldflags at build time.
	Version = "dev"
	Build   = "unknown"
)

func main() {
	root := &cobra.Command{
		Use:   "beads-cost",
		Short: "Cost tracking sidecar for beads",
		Long:  "Track and report LLM session costs. Data lives in .beads/cost.db.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if beadsDir == "" {
				beadsDir = findBeadsDir()
			}
		},
	}

	root.PersistentFlags().StringVar(&beadsDir, "db", "", "path to .beads directory (auto-detected if empty)")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output as JSON")

	root.AddCommand(trackCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(sessionsCmd())
	root.AddCommand(showCmd())
	root.AddCommand(versionCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// findBeadsDir walks up from cwd looking for a .beads directory.
func findBeadsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ".beads"
	}
	for {
		candidate := filepath.Join(dir, ".beads")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ".beads"
}

func openStore() (*cost.Store, error) {
	return cost.Open(beadsDir)
}

// --- track ---

func trackCmd() *cobra.Command {
	var (
		id            string
		beadID        string
		model         string
		provider      string
		inputTokens   int64
		outputTokens  int64
		costUSD       float64
		durationSec   float64
		note          string
		cacheRead     int64
		cacheCreation int64
	)

	cmd := &cobra.Command{
		Use:   "track",
		Short: "Record a session's cost data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if id == "" {
				id = fmt.Sprintf("s-%d", time.Now().UnixNano())
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			sess := cost.Session{
				ID:            id,
				BeadID:        beadID,
				Model:         model,
				Provider:      provider,
				InputTokens:   inputTokens,
				OutputTokens:  outputTokens,
				CostUSD:       costUSD,
				DurationSec:   durationSec,
				Note:          note,
				CacheRead:     cacheRead,
				CacheCreation: cacheCreation,
				CreatedAt:     time.Now().UTC(),
			}
			if err := store.Track(sess); err != nil {
				return fmt.Errorf("tracking session: %w", err)
			}

			if jsonOutput {
				return json.NewEncoder(os.Stdout).Encode(sess)
			}
			fmt.Printf("Tracked session %s (model=%s cost=$%.4f)\n", sess.ID, sess.Model, sess.CostUSD)
			return nil
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "session ID (auto-generated if empty)")
	cmd.Flags().StringVar(&beadID, "bead", "", "associated bead/issue ID")
	cmd.Flags().StringVar(&model, "model", "", "model name (e.g. claude-sonnet-4-5-20250929)")
	cmd.Flags().StringVar(&provider, "provider", "anthropic", "provider name")
	cmd.Flags().Int64Var(&inputTokens, "input", 0, "input token count")
	cmd.Flags().Int64Var(&outputTokens, "output", 0, "output token count")
	cmd.Flags().Float64Var(&costUSD, "cost", 0, "total cost in USD")
	cmd.Flags().Float64Var(&durationSec, "duration", 0, "session duration in seconds")
	cmd.Flags().StringVar(&note, "note", "", "free-form annotation")
	cmd.Flags().Int64Var(&cacheRead, "cache-read", 0, "cache read tokens")
	cmd.Flags().Int64Var(&cacheCreation, "cache-creation", 0, "cache creation tokens")

	return cmd
}

// --- status ---

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show aggregated cost summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			sum, err := store.Status()
			if err != nil {
				return err
			}

			if jsonOutput {
				return json.NewEncoder(os.Stdout).Encode(sum)
			}

			if sum.TotalSessions == 0 {
				fmt.Println("No sessions tracked yet.")
				return nil
			}

			fmt.Printf("Sessions:   %d\n", sum.TotalSessions)
			fmt.Printf("Total cost: $%.4f\n", sum.TotalCostUSD)
			fmt.Printf("Tokens:     %s in / %s out\n", fmtTokens(sum.TotalInput), fmtTokens(sum.TotalOutput))
			fmt.Printf("Duration:   %s\n", fmtDuration(sum.TotalDuration))
			if sum.FirstSession != "" {
				fmt.Printf("Period:     %s .. %s\n", sum.FirstSession[:10], sum.LastSession[:10])
			}
			return nil
		},
	}
}

// --- sessions ---

func sessionsCmd() *cobra.Command {
	var limit int
	var beadID string

	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List tracked sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var sessions []cost.Session
			if beadID != "" {
				sessions, err = store.SessionsForBead(beadID)
			} else {
				sessions, err = store.ListSessions(limit)
			}
			if err != nil {
				return err
			}

			if jsonOutput {
				return json.NewEncoder(os.Stdout).Encode(sessions)
			}

			if len(sessions) == 0 {
				fmt.Println("No sessions found.")
				return nil
			}

			fmt.Printf("%-24s %-10s %-24s %10s %10s %10s\n",
				"ID", "BEAD", "MODEL", "IN", "OUT", "COST")
			fmt.Println(strings.Repeat("-", 92))
			for _, s := range sessions {
				bid := truncate(s.BeadID, 10)
				mid := truncate(s.Model, 24)
				sid := truncate(s.ID, 24)
				fmt.Printf("%-24s %-10s %-24s %10s %10s %10s\n",
					sid, bid, mid,
					fmtTokens(s.InputTokens), fmtTokens(s.OutputTokens),
					fmt.Sprintf("$%.4f", s.CostUSD))
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "max sessions to show")
	cmd.Flags().StringVar(&beadID, "bead", "", "filter by bead ID")
	return cmd
}

// --- show ---

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show details for a single session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			sess, err := store.GetSession(args[0])
			if err != nil {
				return err
			}
			if sess == nil {
				return fmt.Errorf("session %q not found", args[0])
			}

			if jsonOutput {
				return json.NewEncoder(os.Stdout).Encode(sess)
			}

			fmt.Printf("Session:    %s\n", sess.ID)
			if sess.BeadID != "" {
				fmt.Printf("Bead:       %s\n", sess.BeadID)
			}
			fmt.Printf("Model:      %s\n", sess.Model)
			fmt.Printf("Provider:   %s\n", sess.Provider)
			fmt.Printf("Tokens:     %s in / %s out\n", fmtTokens(sess.InputTokens), fmtTokens(sess.OutputTokens))
			if sess.CacheRead > 0 || sess.CacheCreation > 0 {
				fmt.Printf("Cache:      %s read / %s creation\n", fmtTokens(sess.CacheRead), fmtTokens(sess.CacheCreation))
			}
			fmt.Printf("Cost:       $%.4f\n", sess.CostUSD)
			fmt.Printf("Duration:   %s\n", fmtDuration(sess.DurationSec))
			fmt.Printf("Created:    %s\n", sess.CreatedAt.Format(time.RFC3339))
			if sess.Note != "" {
				fmt.Printf("Note:       %s\n", sess.Note)
			}
			return nil
		},
	}
}

// --- version ---

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("beads-cost %s (build %s)\n", Version, Build)
		},
	}
}

// --- helpers ---

func fmtTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func fmtDuration(secs float64) string {
	d := time.Duration(secs * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", secs)
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
