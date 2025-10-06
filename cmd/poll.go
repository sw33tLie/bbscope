package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
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
// Uses global flags from root (proxy, output, delimiter, bbpOnly, pvtOnly, oos, loglevel)
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
	pollCmd.PersistentFlags().String("category", "all", "Scope categories to include (url, cidr, mobile, etc.)")
	pollCmd.PersistentFlags().Bool("db", false, "Save results to the database and print changes")
	pollCmd.PersistentFlags().String("dbpath", "", "Path to SQLite DB file (default: bbscope.sqlite in CWD)")
	pollCmd.PersistentFlags().Int("concurrency", 5, "Number of concurrent program fetches per platform")
	pollCmd.PersistentFlags().String("since", "", "Only print changes since this RFC3339 timestamp (requires --db)")
	pollCmd.PersistentFlags().Bool("oos", false, "Include out-of-scope elements")
	pollCmd.PersistentFlags().StringP("output", "o", "tu", "Output flags. Supported: t (target), d (target description), c (category), u (program URL). Can be combined. Example: -o tdu")
	pollCmd.PersistentFlags().StringP("delimiter", "d", " ", "Delimiter character to use for txt output format")
}

// runPollWithPollers executes the polling flow using the provided pollers.
func runPollWithPollers(cmd *cobra.Command, pollers []platforms.PlatformPoller) error {
	categories, _ := cmd.Flags().GetString("category")
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
		opts := platforms.PollOptions{
			Categories: categories,
		}
		handles, err := p.ListProgramHandles(ctx, opts)
		if err != nil {
			return err
		}
		for _, h := range handles {
			pd, err := p.FetchProgramScope(ctx, h, opts)
			if err != nil {
				return err
			}
			if !useDB {
				output, _ := cmd.Flags().GetString("output")
				delimiter, _ := cmd.Flags().GetString("delimiter")
				oos, _ := cmd.Flags().GetBool("oos")
				scope.PrintProgramScope(pd, output, delimiter, oos)
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
