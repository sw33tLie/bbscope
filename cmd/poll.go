package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	h1platform "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
	testplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/test"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// pollCmd implements: bbscope poll
// Flags (from REFACTOR.md):
//
//	--platform string   Comma-separated platforms or "all" (default)
//	--program string    Filter by program (handle or full URL)
//	--db                Persist results to the database
//	--dry-run           Simulate without writing to DB
//	--concurrency int   Number of concurrent fetches
//	--since string      Print changes since RFC3339 timestamp (when using --db)
//
// Uses global flags from root (proxy, output, delimiter, bbpOnly, pvtOnly, oos, loglevel)
var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll platforms and fetch scopes",
	RunE: func(cmd *cobra.Command, _ []string) error {
		platformsFlag, _ := cmd.Flags().GetString("platform")
		enabled := map[string]bool{}
		if platformsFlag == "all" || platformsFlag == "" {
			enabled["test"] = true
		} else {
			for _, p := range strings.Split(platformsFlag, ",") {
				enabled[strings.ToLower(strings.TrimSpace(p))] = true
			}
		}
		var pollers []platforms.PlatformPoller
		if enabled["test"] {
			pollers = append(pollers, &testplatform.Poller{})
		}
		if enabled["h1"] || enabled["hackerone"] {
			user, _ := cmd.Flags().GetString("h1-user")
			token, _ := cmd.Flags().GetString("h1-token")
			if user == "" || token == "" {
				return fmt.Errorf("hackerone requires --h1-user and --h1-token")
			}
			pollers = append(pollers, h1platform.NewPoller(user, token))
		}
		if len(pollers) == 0 {
			return fmt.Errorf("no platforms selected")
		}
		return runPollWithPollers(cmd, pollers)
	},
}

func init() {
	rootCmd.AddCommand(pollCmd)

	pollCmd.Flags().String("platform", "all", "Comma-separated platforms to poll (h1,bc,it,ywh,immunefi) or 'all'")
	// Make common flags persistent so subcommands inherit them
	pollCmd.PersistentFlags().String("program", "", "Filter by program handle or full URL")
	pollCmd.PersistentFlags().Bool("db", false, "Persist results to the database and print changes")
	pollCmd.PersistentFlags().String("dbpath", "", "Path to SQLite DB file (default: bbscope.sqlite in CWD)")
	// HackerOne auth flags (temporary; will move to config)
	pollCmd.Flags().String("h1-user", "", "HackerOne username (required for h1)")
	pollCmd.Flags().String("h1-token", "", "HackerOne API token (required for h1)")

	pollCmd.PersistentFlags().Bool("dry-run", false, "Simulate actions without writing to the database")
	pollCmd.PersistentFlags().Int("concurrency", 5, "Number of concurrent program fetches per platform")
	pollCmd.PersistentFlags().String("since", "", "Only print changes since this RFC3339 timestamp (requires --db)")
}

// runPollWithPollers executes the polling flow using the provided pollers.
func runPollWithPollers(cmd *cobra.Command, pollers []platforms.PlatformPoller) error {
	programFilter, _ := cmd.Flags().GetString("program")
	useDB, _ := cmd.Flags().GetBool("db")
	dbPath, _ := cmd.Flags().GetString("dbpath")
	if dbPath == "" {
		dbPath = "bbscope.sqlite"
	}

	var db *storage.DB
	var err error
	if useDB {
		db, err = storage.Open(dbPath)
		if err != nil {
			return err
		}
		defer db.Close()
	}

	ctx := context.Background()
	for _, p := range pollers {
		proxy, _ := cmd.Flags().GetString("proxy")
		cfg := platforms.AuthConfig{
			Proxy: proxy,
		}

		if err := p.Authenticate(ctx, cfg); err != nil {
			return fmt.Errorf("authentication failed for %s: %w", p.Name(), err)
		}

		opts := platforms.PollOptions{ProgramFilter: programFilter}
		handles, err := p.ListProgramHandles(ctx, opts)
		if err != nil {
			return err
		}
		for _, h := range handles {
			if programFilter != "" && !strings.Contains(h, programFilter) {
				continue
			}
			pd, err := p.FetchProgramScope(ctx, h, opts)
			if err != nil {
				return err
			}
			if !useDB {
				scope.PrintProgramScope(pd, "tu", " ", true)
				continue
			}

			var allItems []storage.TargetItem
			for _, s := range pd.InScope {
				allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: true})
			}
			for _, s := range pd.OutOfScope {
				allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: false})
			}

			entries, err := storage.BuildEntries(pd.Url, p.Name(), h, allItems)
			if err != nil {
				return err
			}
			changes, err := db.UpsertProgramEntries(ctx, pd.Url, p.Name(), h, entries)
			if err != nil {
				return err
			}
			for _, c := range changes {
				fmt.Printf("[change] %s %s %s %s in_scope=%t\n", c.ChangeType, c.Platform, c.ProgramURL, c.TargetNormalized, c.InScope)
			}
		}
	}
	return nil
}

func getPollerNames(pollers []platforms.PlatformPoller) []string {
	names := make([]string, len(pollers))
	for i, p := range pollers {
		names[i] = p.Name()
	}
	return names
}
