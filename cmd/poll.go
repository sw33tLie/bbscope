package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/ai"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	bcplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/bugcrowd"
	h1platform "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
	itplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/intigriti"
	ywhplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/yeswehack"
	"github.com/sw33tLie/bbscope/v2/pkg/polling"
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

		proxyURL, _ := cmd.Flags().GetString("proxy")
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
			authCfg := platforms.AuthConfig{Email: bcEmail, Password: bcPass, OtpSecret: bcOTP, Proxy: proxyURL}
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
			if err := itPoller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: itToken, Proxy: proxyURL}); err != nil {
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
			authCfg := platforms.AuthConfig{Email: ywhEmail, Password: ywhPass, OtpSecret: ywhOTP, Proxy: proxyURL}
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

	bbpOnly, _ := cmd.Flags().GetBool("bbp-only")
	pvtOnly, _ := cmd.Flags().GetBool("private-only")
	opts := platforms.PollOptions{
		Categories:  categories,
		BountyOnly:  bbpOnly,
		PrivateOnly: pvtOnly,
	}

	if !useDB {
		return runPollNoDB(cmd, pollers, opts)
	}

	dbURL, err := GetDBConnectionString()
	if err != nil {
		return err
	}
	db, err := storage.Open(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	var aiNormalizer ai.Normalizer
	if useAI {
		proxyURL, _ := rootCmd.Flags().GetString("proxy")
		cfg := ai.Config{
			Provider:       strings.TrimSpace(viper.GetString("ai.provider")),
			APIKey:         strings.TrimSpace(viper.GetString("ai.api_key")),
			Model:          strings.TrimSpace(viper.GetString("ai.model")),
			MaxBatch:       viper.GetInt("ai.max_batch"),
			MaxConcurrency: viper.GetInt("ai.max_concurrency"),
			Endpoint:       strings.TrimSpace(viper.GetString("ai.endpoint")),
			Proxy:          strings.TrimSpace(viper.GetString("ai.proxy")),
		}
		if proxyURL != "" {
			cfg.Proxy = proxyURL
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

	for _, p := range pollers {
		utils.Log.Infof("Fetching scope from %s...", p.Name())

		result, err := polling.PollPlatform(ctx, polling.PlatformConfig{
			Poller:      p,
			Options:     opts,
			DB:          db,
			Concurrency: concurrency,
			Normalizer:  aiNormalizer,
			Log:         utils.Log,
			OnProgramDone: func(programURL string, changes []storage.Change, isFirstRun bool) {
				if !isFirstRun {
					printChanges(changes)
				}
			},
		})
		if err != nil {
			return err
		}

		if !result.IsFirstRun {
			printChanges(result.RemovedProgramChanges)
		}

		if len(result.Errors) > 0 {
			return result.Errors[0]
		}
	}
	return nil
}

// runPollNoDB handles the non-DB polling mode: fetch and print scope directly.
func runPollNoDB(cmd *cobra.Command, pollers []platforms.PlatformPoller, opts platforms.PollOptions) error {
	useAI, _ := cmd.Flags().GetBool("ai")
	if useAI {
		utils.Log.Warn("--ai flag currently only affects --db workflows; enable --db to persist normalized results")
	}

	ctx := context.Background()
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = 5
	}

	output, _ := cmd.Flags().GetString("output")
	delimiter, _ := cmd.Flags().GetString("delimiter")
	oos, _ := cmd.Flags().GetBool("oos")

	for _, p := range pollers {
		utils.Log.Infof("Fetching scope from %s...", p.Name())

		handles, err := p.ListProgramHandles(ctx, opts)
		if err != nil {
			return err
		}

		handleChan := make(chan string, len(handles))
		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for h := range handleChan {
					pd, err := p.FetchProgramScope(ctx, h, opts)
					if err != nil {
						utils.Log.Warnf("Failed to fetch scope for %s: %v", h, err)
						continue
					}
					scope.PrintProgramScope(pd, output, delimiter, oos)
				}
			}()
		}
		for _, h := range handles {
			handleChan <- h
		}
		close(handleChan)
		wg.Wait()
	}
	return nil
}

func printChanges(changes []storage.Change) {
	// Track which targets have variant changes (AI-normalized)
	hasVariants := make(map[string]bool)
	for _, c := range changes {
		if c.TargetAINormalized != "" {
			key := fmt.Sprintf("%s|%s|%s", c.Platform, c.ProgramURL, c.TargetRaw)
			hasVariants[key] = true
		}
	}

	for _, c := range changes {
		// Skip base target changes if there are variant changes for the same target
		if c.TargetAINormalized == "" {
			key := fmt.Sprintf("%s|%s|%s", c.Platform, c.ProgramURL, c.TargetRaw)
			if hasVariants[key] {
				continue
			}
		}

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
