package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/ai"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	bcplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/bugcrowd"
	h1platform "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
	itplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/intigriti"
	ywhplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/yeswehack"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// pollCmd implements: bbscope poll
// Flags (from REFACTOR.md):
//
//	--platform string   Comma-separated platforms or "all" (default)
//	--program string    Filter by program (handle or full URL)
//	--db                Save results to the database
//	--concurrency int   Number of concurrent fetches
//	--since string      Print changes since RFC3339 timestamp (when using --db)
//
// Uses global flags from root (proxy, output, delimiter, bbp-only, private-only, oos, loglevel)
var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Poll platforms and fetch scopes",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown command: '%s'. See 'bbscope poll --help'", args[0])
		}

		proxy, _ := cmd.Flags().GetString("proxy")
		var pollers []platforms.PlatformPoller

		// H1
		h1User := viper.GetString("hackerone.username")
		h1Token := viper.GetString("hackerone.token")
		if h1User != "" && h1Token != "" {
			h1Poller := h1platform.NewPoller(h1User, h1Token)
			pollers = append(pollers, h1Poller)
		} else {
			utils.Log.Info("Skipping HackerOne: credentials not found in config.")
		}

		// Bugcrowd
		bcEmail := viper.GetString("bugcrowd.email")
		bcPass := viper.GetString("bugcrowd.password")
		bcOTP := viper.GetString("bugcrowd.otpsecret")
		if bcEmail != "" && bcPass != "" && bcOTP != "" {
			bcPoller := &bcplatform.Poller{}
			authCfg := platforms.AuthConfig{Email: bcEmail, Password: bcPass, OtpSecret: bcOTP, Proxy: proxy}
			if err := bcPoller.Authenticate(cmd.Context(), authCfg); err != nil {
				utils.Log.Errorf("Bugcrowd auth failed: %v", err)
			} else {
				pollers = append(pollers, bcPoller)
			}
		} else {
			utils.Log.Info("Skipping Bugcrowd: email, password, or otpsecret not found in config.")
		}

		// Intigriti
		itToken := viper.GetString("intigriti.token")
		if itToken != "" {
			itPoller := itplatform.NewPoller()
			if err := itPoller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: itToken, Proxy: proxy}); err != nil {
				utils.Log.Errorf("Intigriti auth failed: %v", err)
			} else {
				pollers = append(pollers, itPoller)
			}
		} else {
			utils.Log.Info("Skipping Intigriti: token not found in config.")
		}

		// YesWeHack
		ywhEmail := viper.GetString("yeswehack.email")
		ywhPass := viper.GetString("yeswehack.password")
		ywhOTP := viper.GetString("yeswehack.otpsecret")
		if ywhEmail != "" && ywhPass != "" && ywhOTP != "" {
			ywhPoller := &ywhplatform.Poller{}
			authCfg := platforms.AuthConfig{Email: ywhEmail, Password: ywhPass, OtpSecret: ywhOTP, Proxy: proxy}
			if err := ywhPoller.Authenticate(cmd.Context(), authCfg); err != nil {
				utils.Log.Errorf("YesWeHack auth failed: %v", err)
			} else {
				pollers = append(pollers, ywhPoller)
			}
		} else {
			utils.Log.Info("Skipping YesWeHack: email, password, or otpsecret not found in config.")
		}

		if len(pollers) == 0 {
			utils.Log.Info("No platforms to poll. Configure credentials in ~/.bbscope.yaml")
			return nil
		}

		return runPollWithPollers(cmd, pollers)
	},
}

func init() {
	rootCmd.AddCommand(pollCmd)

	// Make common flags persistent so subcommands inherit them
	pollCmd.PersistentFlags().String("category", "all", "Scope categories to include (wildcard, url, cidr, apple, android, ai, etc.)")
	pollCmd.PersistentFlags().Bool("db", false, "Save results to the database and print changes")
	pollCmd.PersistentFlags().String("dbpath", "", "Path to SQLite DB file (default: bbscope.sqlite in CWD)")
	pollCmd.PersistentFlags().Int("concurrency", 5, "Number of concurrent program fetches per platform")
	pollCmd.PersistentFlags().String("since", "", "Only print changes since this RFC3339 timestamp (requires --db)")
	pollCmd.PersistentFlags().Bool("oos", false, "Include out-of-scope elements")
	pollCmd.PersistentFlags().StringP("output", "o", "tu", "Output flags. Supported: t (target), d (target description), c (category), u (program URL). Can be combined. Example: -o tdu")
	pollCmd.PersistentFlags().StringP("delimiter", "d", " ", "Delimiter character to use for txt output format")
	pollCmd.PersistentFlags().BoolP("bbp-only", "b", false, "Only fetch programs offering monetary rewards")
	pollCmd.PersistentFlags().BoolP("private-only", "p", false, "Only fetch data from private programs")
	pollCmd.PersistentFlags().Bool("ai", false, "Enable LLM-assisted normalization (requires ai.api_key or OPENAI_API_KEY)")
}

// runPollWithPollers executes the polling flow using the provided pollers.
func runPollWithPollers(cmd *cobra.Command, pollers []platforms.PlatformPoller) error {
	categories, _ := cmd.Flags().GetString("category")
	useDB, _ := cmd.Flags().GetBool("db")
	useAI, _ := cmd.Flags().GetBool("ai")
	dbPath, _ := cmd.Flags().GetString("dbpath")
	if dbPath == "" {
		dbPath = "bbscope.sqlite"
	}

	var db *storage.DB
	var err error
	if useDB {
		db, err = storage.Open(dbPath, storage.DefaultDBTimeout)
		if err != nil {
			return err
		}
		defer db.Close()
	}

	if useAI && !useDB {
		utils.Log.Warn("--ai flag currently only affects --db workflows; enable --db to persist normalized results")
	}

	var aiNormalizer ai.Normalizer
	if useAI {
		cfg := ai.Config{
			Provider:       strings.TrimSpace(viper.GetString("ai.provider")),
			APIKey:         strings.TrimSpace(viper.GetString("ai.api_key")),
			Model:          strings.TrimSpace(viper.GetString("ai.model")),
			MaxBatch:       viper.GetInt("ai.max_batch"),
			MaxConcurrency: viper.GetInt("ai.max_concurrency"),
			Endpoint:       strings.TrimSpace(viper.GetString("ai.endpoint")),
		}
		if cfg.APIKey == "" {
			cfg.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}

		normalizer, err := ai.NewNormalizer(cfg)
		if err != nil {
			return err
		}
		aiNormalizer = normalizer
	}

	ctx := context.Background()
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = 5 // Default to 5 if invalid
	}

	for _, p := range pollers {
		utils.Log.Infof("Fetching scope from %s...", p.Name())

		bbpOnly, _ := cmd.Flags().GetBool("bbp-only")
		pvtOnly, _ := cmd.Flags().GetBool("private-only")
		opts := platforms.PollOptions{
			Categories:  categories,
			BountyOnly:  bbpOnly,
			PrivateOnly: pvtOnly,
		}

		isFirstRunForPlatform := false
		if useDB {
			programCount, err := db.GetActiveProgramCount(ctx, p.Name())
			if err != nil {
				// Don't fail the whole run, but we can't do the "first run" check.
				utils.Log.Warnf("Could not get program count for %s: %v", p.Name(), err)
			} else {
				isFirstRunForPlatform = programCount == 0
			}
		}

		var ignoredPrograms map[string]bool
		if useDB {
			var err error
			ignoredPrograms, err = db.GetIgnoredPrograms(ctx, p.Name())
			if err != nil {
				utils.Log.Warnf("Could not get ignored programs for %s: %v", p.Name(), err)
				ignoredPrograms = make(map[string]bool) // Continue with an empty map
			}
		}

		handles, err := p.ListProgramHandles(ctx, opts)
		if err != nil {
			return err
		}

		if isFirstRunForPlatform && len(handles) > 0 {
			utils.Log.Infof("First poll for %s, populating database...", p.Name())
		}

		if useDB {
			dbProgramCount, err := db.GetActiveProgramCount(ctx, p.Name())
			if err != nil {
				utils.Log.Warnf("Could not get program count for %s: %v", p.Name(), err)
			}

			// PLATFORM-LEVEL SAFETY CHECK: If the poller returns 0 programs, but we have many in the DB,
			// it's likely the poller failed or there's a temporary API issue. We abort the sync
			// for this platform to prevent wiping all its programs.
			if len(handles) == 0 && dbProgramCount > 10 { // Using a threshold > 10
				utils.Log.Errorf("Poller for %s returned 0 programs, but database has %d. Aborting sync for this platform to prevent data loss.", p.Name(), dbProgramCount)
				continue // Skip to the next platform
			}
		}

		// Use concurrent processing with worker pool pattern
		polledProgramURLs, err := processProgramsConcurrently(ctx, cmd, p, handles, opts, useDB, db, ignoredPrograms, isFirstRunForPlatform, concurrency, aiNormalizer)
		if err != nil {
			return err
		}

		if useDB {
			// After processing all programs for a platform, sync the state.
			// This will mark any programs that were not in the latest poll as disabled.
			var removedProgramChanges []storage.Change
			const maxRetries = 5
			const initialBackoff = 1 * time.Second

			err := executeDBWrite(maxRetries, initialBackoff, func() error {
				var err error
				removedProgramChanges, err = db.SyncPlatformPrograms(ctx, p.Name(), polledProgramURLs)
				return err
			})

			if err != nil {
				// We can log this as a warning instead of returning a fatal error
				utils.Log.Warnf("Failed to sync removed programs for platform %s: %v", p.Name(), err)
			}
			if !isFirstRunForPlatform {
				printChanges(removedProgramChanges)
			}
			if err := db.LogChanges(ctx, removedProgramChanges); err != nil {
				utils.Log.Warnf("Could not log removed program changes for platform %s: %v", p.Name(), err)
			}
		}
	}
	return nil
}

// processProgramsConcurrently processes programs using a worker pool pattern for concurrent fetching.
func processProgramsConcurrently(ctx context.Context, cmd *cobra.Command, p platforms.PlatformPoller, handles []string, opts platforms.PollOptions, useDB bool, db *storage.DB, ignoredPrograms map[string]bool, isFirstRunForPlatform bool, concurrency int, aiNormalizer ai.Normalizer) ([]string, error) {
	if len(handles) == 0 {
		return []string{}, nil
	}

	// Channel to distribute work
	handleChan := make(chan string, len(handles))

	// Results collection with mutex protection
	var mu sync.Mutex
	polledProgramURLs := make([]string, 0, len(handles))
	var firstError error
	var errorMu sync.Mutex

	// Worker pool
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range handleChan {
				pd, err := p.FetchProgramScope(ctx, h, opts)
				if err != nil {
					// Log error but continue processing other programs
					utils.Log.Warnf("Failed to fetch scope for %s: %v", h, err)
					errorMu.Lock()
					if firstError == nil {
						firstError = err // Store first error but don't stop processing
					}
					errorMu.Unlock()
					continue
				}

				if useDB && ignoredPrograms[pd.Url] {
					utils.Log.Debugf("Skipping ignored program: %s", pd.Url)
					continue
				}

				// Add to polled URLs (thread-safe)
				mu.Lock()
				polledProgramURLs = append(polledProgramURLs, pd.Url)
				mu.Unlock()

				if !useDB {
					output, _ := cmd.Flags().GetString("output")
					delimiter, _ := cmd.Flags().GetString("delimiter")
					oos, _ := cmd.Flags().GetBool("oos")
					scope.PrintProgramScope(pd, output, delimiter, oos)
					continue
				}

				// Process database operations
				var allItems []storage.TargetItem
				for _, s := range pd.InScope {
					allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: true})
				}
				for _, s := range pd.OutOfScope {
					allItems = append(allItems, storage.TargetItem{URI: s.Target, Category: s.Category, Description: s.Description, InScope: false})
				}

				processedItems := allItems
				if aiNormalizer != nil && len(allItems) > 0 {
					normalized, err := aiNormalizer.NormalizeTargets(ctx, ai.ProgramInfo{
						ProgramURL: pd.Url,
						Platform:   p.Name(),
						Handle:     h,
					}, allItems)
					if err != nil {
						utils.Log.Warnf("AI normalization failed for %s: %v", pd.Url, err)
					} else if len(normalized) > 0 {
						processedItems = normalized
					}
				}

				entries, err := storage.BuildEntries(pd.Url, p.Name(), h, processedItems)
				if err != nil {
					errorMu.Lock()
					if firstError == nil {
						firstError = err
					}
					errorMu.Unlock()
					continue
				}

				const maxRetries = 5
				const initialBackoff = 1 * time.Second

				var changes []storage.Change
				err = executeDBWrite(maxRetries, initialBackoff, func() error {
					var err error
					changes, err = db.UpsertProgramEntries(ctx, pd.Url, p.Name(), h, entries)
					return err
				})

				if err != nil {
					if errors.Is(err, storage.ErrAbortingScopeWipe) {
						utils.Log.Warnf("Potential scope wipe detected for program %s. Skipping update. This might be due to a broken poller or a platform API change.", pd.Url)
						continue // Don't treat this as a fatal error for the whole poll
					}
					// For other errors, log but continue processing
					utils.Log.Warnf("Database error for program %s: %v", pd.Url, err)
					errorMu.Lock()
					if firstError == nil {
						firstError = err
					}
					errorMu.Unlock()
					continue
				}

				// Print changes (thread-safe - fmt.Printf is safe for concurrent use)
				if !isFirstRunForPlatform {
					printChanges(changes)
				}
				if err := db.LogChanges(ctx, changes); err != nil {
					utils.Log.Warnf("Could not log changes for program %s: %v", pd.Url, err)
				}
			}
		}()
	}

	// Send all handles to the channel
	for _, h := range handles {
		handleChan <- h
	}
	close(handleChan)

	// Wait for all workers to finish
	wg.Wait()

	// Return first error if any occurred, but still return the results
	// This allows partial success - some programs may have been processed successfully
	return polledProgramURLs, firstError
}

// executeDBWrite handles retry logic for database write operations that might fail
// due to locking. It uses exponential backoff.
func executeDBWrite(maxRetries int, initialBackoff time.Duration, op func() error) error {
	var err error
	backoff := initialBackoff
	for i := 0; i < maxRetries; i++ {
		err = op()
		if err == nil {
			return nil
		}

		// Check if the error is a SQLite busy/locked error. This is driver-specific.
		// For modernc.org/sqlite, the error message contains "database is locked".
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			utils.Log.Debugf("Database is locked, retrying in %s... (attempt %d/%d)", backoff, i+1, maxRetries)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
			continue
		}

		// For other types of errors, don't retry.
		return err
	}
	return fmt.Errorf("database operation failed after %d retries: %w", maxRetries, err)
}

func printChanges(changes []storage.Change) {
	for _, c := range changes {
		var emoji string
		switch c.ChangeType {
		case "added":
			emoji = "🆕"
		case "removed":
			// Special case for entire program removals
			if c.Category == "program" {
				fmt.Printf("❌ Program removed: %s\n", c.ProgramURL)
				continue
			}
			emoji = "❌"
		case "updated":
			emoji = "🔄"
		}

		scopeStatus := ""
		if !c.InScope {
			scopeStatus = " [OOS]"
		}
		targetDisplay := c.TargetRaw
		if targetDisplay == "" {
			targetDisplay = c.TargetNormalized
		}
		if c.TargetAINormalized != "" {
			targetDisplay = fmt.Sprintf("%s -> %s", targetDisplay, c.TargetAINormalized)
		}
		fmt.Printf("%s  %s  %s  %s%s\n", emoji, c.Platform, c.ProgramURL, targetDisplay, scopeStatus)
	}
}
