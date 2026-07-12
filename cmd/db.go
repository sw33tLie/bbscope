package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Interact with the bbscope database",
}

// statsCmd represents the stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Prints statistics about the programs and assets in the database.",
	Long:  "Prints statistics about the programs and assets in the database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}

		db, err := storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()

		stats, err := db.GetStats(context.Background(), "")
		if err != nil {
			return err
		}

		if len(stats) == 0 {
			fmt.Println("No data in the database to generate stats.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.AlignRight)
		fmt.Fprintln(w, "PLATFORM\tPROGRAMS\tIN-SCOPE\tOUT-OF-SCOPE\t")

		var totalPrograms, totalInScope, totalOutOfScope int
		for _, s := range stats {
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t\n", s.Platform, s.ProgramCount, s.InScopeCount, s.OutOfScopeCount)
			totalPrograms += s.ProgramCount
			totalInScope += s.InScopeCount
			totalOutOfScope += s.OutOfScopeCount
		}

		fmt.Fprintln(w, " \t \t \t \t")
		fmt.Fprintf(w, "TOTAL\t%d\t%d\t%d\t\n", totalPrograms, totalInScope, totalOutOfScope)

		w.Flush()

		return nil
	},
}

var changesCmd = &cobra.Command{
	Use:   "changes",
	Short: "Show recent scope changes (default 50)",
	Long: `Show recent scope changes. Use --since and --until to filter by time range.

Supported time formats for --since and --until:
  today, yesterday, 7d, 30d, 90d, 1y, or YYYY-MM-DD`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		sinceStr, _ := cmd.Flags().GetString("since")
		untilStr, _ := cmd.Flags().GetString("until")
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}

		db, err := storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()

		since, err := parseTimeFlag(sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		until, err := parseTimeFlag(untilStr)
		if err != nil {
			return fmt.Errorf("invalid --until value: %w", err)
		}
		// When --until is a date, include the full day
		if !until.IsZero() && untilStr != "" && !strings.Contains(untilStr, "d") && untilStr != "today" && untilStr != "yesterday" {
			until = until.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		}

		changes, err := db.ListRecentChanges(context.Background(), limit, since, until)
		if err != nil {
			return err
		}
		for _, c := range changes {
			ts := c.OccurredAt.Format("2006-01-02 15:04:05")

			if c.ChangeType == "removed" && c.Category == "program" {
				fmt.Printf("%s  %-6s  %s  Program removed: %s\n", ts, c.ChangeType, c.Platform, c.ProgramURL)
				continue
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
			fmt.Printf("%s  %-6s  %s  %s  %s%s\n", ts, c.ChangeType, c.Platform, c.ProgramURL, targetDisplay, scopeStatus)
		}
		return nil
	},
}

// parseTimeFlag parses a user-friendly time string into a time.Time.
// Supports: today, yesterday, 7d, 30d, 90d, 1y, YYYY-MM-DD, or empty string.
func parseTimeFlag(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	now := time.Now()
	switch s {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "yesterday":
		y := now.AddDate(0, 0, -1)
		return time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, y.Location()), nil
	case "7d":
		return now.AddDate(0, 0, -7), nil
	case "30d":
		return now.AddDate(0, 0, -30), nil
	case "90d":
		return now.AddDate(0, 0, -90), nil
	case "1y":
		return now.AddDate(-1, 0, 0), nil
	default:
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("use today, yesterday, 7d, 30d, 90d, 1y, or YYYY-MM-DD")
		}
		return t, nil
	}
}

var printCmd = &cobra.Command{
	Use:   "print",
	Short: "Print scope data from the database",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		platform, _ := cmd.Flags().GetString("platform")
		program, _ := cmd.Flags().GetString("program")
		oos, _ := cmd.Flags().GetBool("oos")
		sinceStr, _ := cmd.Flags().GetString("since")
		format, _ := cmd.Flags().GetString("format")
		includeIgnored, _ := cmd.Flags().GetBool("include-ignored")
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}

		db, err := storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()

		var since time.Time
		if sinceStr != "" {
			s, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return fmt.Errorf("invalid --since, need RFC3339: %w", err)
			}
			since = s
		}

		entries, err := db.ListEntries(context.Background(), storage.ListOptions{
			Platform:       platform,
			ProgramFilter:  program,
			Since:          since,
			IncludeOOS:     oos,
			IncludeIgnored: includeIgnored,
		})
		if err != nil {
			return err
		}

		// Basic type filtering for urls/wildcards/apis/mobile
		filtered := entries
		/*
			for _, e := range entries {
				switch typ {
				case "all":
					filtered = append(filtered, e)
				case "urls":
					if strings.HasPrefix(e.TargetNormalized, "http://") || strings.HasPrefix(e.TargetNormalized, "https://") {
						filtered = append(filtered, e)
					}
				case "wildcards":
					if strings.HasPrefix(e.TargetNormalized, "*.") {
						filtered = append(filtered, e)
					}
				case "apis":
					if e.Category == "api" {
						filtered = append(filtered, e)
					}
				case "mobile":
					if e.Category == "android" || e.Category == "ios" {
						filtered = append(filtered, e)
					}
				default:
					// unknown type: return error
					return fmt.Errorf("unknown type: %s", typ)
				}
			}
		*/

		switch format {
		case "txt":
			output, _ := cmd.Flags().GetString("output")
			delimiter, _ := cmd.Flags().GetString("delimiter")

			// Reuse existing printer by converting back into ProgramData groups
			byProgram := map[string][]storage.Entry{}
			for _, e := range filtered {
				byProgram[e.ProgramURL] = append(byProgram[e.ProgramURL], e)
			}
			for url, list := range byProgram {
				pd := scope.ProgramData{Url: url}
				for _, e := range list {
					se := scope.ScopeElement{Target: e.TargetNormalized, Description: e.Description, Category: e.Category}
					if e.InScope {
						pd.InScope = append(pd.InScope, se)
					} else if oos {
						pd.OutOfScope = append(pd.OutOfScope, se)
					}
				}
				scope.PrintProgramScope(pd, output, delimiter, oos)
			}
		case "json":
			out := make([]interface{}, 0)
			for _, e := range filtered {
				out = append(out, struct {
					ProgramURL  string `json:"program_url"`
					Platform    string `json:"platform"`
					Handle      string `json:"handle"`
					Target      string `json:"target"`
					Category    string `json:"category"`
					Description string `json:"description"`
					InScope     bool   `json:"in_scope"`
				}{
					ProgramURL:  e.ProgramURL,
					Platform:    e.Platform,
					Handle:      e.Handle,
					Target:      e.TargetNormalized,
					Category:    e.Category,
					Description: e.Description,
					InScope:     e.InScope,
				})
			}
			bytes, err := json.Marshal(out)
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
		case "csv":
			fmt.Println("program_url,platform,handle,target,category,description,in_scope")
			for _, e := range filtered {
				// naive CSV, no quoting for commas in description
				fmt.Printf("%s,%s,%s,%s,%s,%s,%t\n", e.ProgramURL, e.Platform, e.Handle, e.TargetNormalized, e.Category, strings.ReplaceAll(e.Description, ",", " "), e.InScope)
			}
		default:
			return fmt.Errorf("unknown format: %s", format)
		}

		return nil
	},
}

var findCmd = &cobra.Command{
	Use:   "find [query]",
	Short: "Search for a string in current and historical scopes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		searchTerm := args[0]
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}

		db, err := storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()

		results, err := db.SearchTargets(context.Background(), searchTerm)
		if err != nil {
			return err
		}

		if len(results) == 0 {
			fmt.Println("No results found.")
			return nil
		}

		// Simple text output for now
		for _, e := range results {
			var inScopeStatus string
			if e.InScope {
				inScopeStatus = "in-scope"
			} else {
				inScopeStatus = "out-of-scope"
			}
			historicalTag := ""
			if e.IsHistorical {
				historicalTag = " (historical)"
			}
			fmt.Printf("%s | %s | %s (%s)%s\n", e.Platform, e.ProgramURL, e.TargetNormalized, inScopeStatus, historicalTag)
		}

		return nil
	},
}

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open a psql shell to the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}

		fmt.Printf("Connecting to %s...\n", dbURL)
		fmt.Println("bbscope database schema:")
		fmt.Println("  programs      (id, platform, handle, url, disabled, is_ignored, last_seen_at)")
		fmt.Println("  targets_raw   (id, program_id, target, category, in_scope, is_bbp, description, last_seen_at)")
		fmt.Println("  scope_changes (id, program_url, platform, change_type, target_raw, target_normalized, category, is_bbp, occurred_at)")
		fmt.Println("  targets_ai_enhanced (id, target_id, target_ai_normalized, category, in_scope)")
		fmt.Println("")

		pgCmd := exec.Command("psql", dbURL)
		pgCmd.Stdin = os.Stdin
		pgCmd.Stdout = os.Stdout
		pgCmd.Stderr = os.Stderr

		if err := pgCmd.Run(); err != nil {
			return fmt.Errorf("psql exited with error: %w", err)
		}

		return nil
	},
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a custom target to the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("target")
		category, _ := cmd.Flags().GetString("category")
		programURL, _ := cmd.Flags().GetString("program-url")
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}

		if target == "" {
			return errors.New("target flag is required")
		}

		db, err := storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()

		targets := strings.Split(target, ",")
		for _, t := range targets {
			t = strings.TrimSpace(t)
			if t != "" {
				created, err := db.AddCustomTarget(context.Background(), t, category, programURL)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error adding target %s: %v\n", t, err)
				} else {
					if created {
						fmt.Printf("Successfully added target: %s\n", t)
					} else {
						fmt.Printf("Target already exists, refreshed timestamp: %s\n", t)
					}
				}
			}
		}
		return nil
	},
}

var programMetaCmd = &cobra.Command{
	Use:   "program <platform>/<handle>",
	Short: "Show full metadata for a single program (rewards, qualifying vulnerabilities, reports stats, etc.)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}
		db, err := storage.Open(dbURL)
		if err != nil {
			return err
		}
		defer db.Close()

		key := args[0]
		idx := strings.Index(key, "/")
		if idx <= 0 {
			return fmt.Errorf("expected <platform>/<handle>, got %q", key)
		}
		platform := key[:idx]
		handle := key[idx+1:]

		prog, err := db.GetProgramByPlatformHandleAny(context.Background(), platform, handle)
		if err != nil {
			return err
		}
		if prog == nil {
			return fmt.Errorf("program %s/%s not found", platform, handle)
		}

		md, err := db.GetProgramMetadata(context.Background(), prog.ID)
		if err != nil {
			return err
		}
		if md == nil {
			fmt.Println("No metadata stored for this program.")
			return nil
		}

		fmt.Printf("Program: %s\n", prog.URL)
		if md.Title != "" {
			fmt.Printf("Title: %s\n", md.Title)
		}
		if md.Tagline != "" {
			fmt.Printf("Tagline: %s\n", md.Tagline)
		}
		if md.CompanyName != "" {
			fmt.Printf("Company: %s\n", md.CompanyName)
		}
		if md.Industry != "" {
			fmt.Printf("Industry: %s\n", md.Industry)
		}
		if md.ProgramType != "" {
			fmt.Printf("Type: %s\n", md.ProgramType)
		}
		fmt.Printf("Bounty: %s  VDP: %s  Public: %s\n",
			boolOrDash(md.IsBounty), boolOrDash(md.IsVDP), boolOrDash(md.IsPublic))
		if md.Secured != nil {
			fmt.Printf("2FA required: %v\n", *md.Secured)
		}
		if md.IsDisabled != nil && *md.IsDisabled {
			fmt.Printf("Status: disabled/paused\n")
		}
		if md.SafeHarbor != "" {
			fmt.Printf("Safe harbor: %s\n", md.SafeHarbor)
		}

		if md.HasRewardInfo() {
			fmt.Println("\nRewards:")
			if md.Currency != "" {
				fmt.Printf("  Currency: %s\n", md.Currency)
			}
			if md.BountyRewardMin != nil {
				fmt.Printf("  Min: %d %s\n", *md.BountyRewardMin, md.Currency)
			}
			if md.BountyRewardMax != nil {
				fmt.Printf("  Max: %d %s\n", *md.BountyRewardMax, md.Currency)
			}
			if len(md.RewardGrids) > 0 {
				fmt.Println("  Reward grids:")
				for _, g := range md.RewardGrids {
					fmt.Printf("    [%s]  info=%s  low=%s  medium=%s  high=%s  critical=%s  exceptional=%s\n",
						g.Dimension,
						scope.FormatBountySlot(g.BountyInfoMin, g.BountyInfoMax),
						scope.FormatBountySlot(g.BountyLowMin, g.BountyLowMax),
						scope.FormatBountySlot(g.BountyMediumMin, g.BountyMediumMax),
						scope.FormatBountySlot(g.BountyHighMin, g.BountyHighMax),
						scope.FormatBountySlot(g.BountyCriticalMin, g.BountyCriticalMax),
						scope.FormatBountySlot(g.BountyExceptionalMin, g.BountyExceptionalMax))
				}
			}
		}

		if md.ReportsCount != nil || md.TotalPayout != nil || md.AvgReward != nil {
			fmt.Println("\nReports:")
			if md.ReportsCount != nil {
				fmt.Printf("  Total: %d\n", *md.ReportsCount)
			}
			if md.TotalPayout != nil {
				fmt.Printf("  Total payout: %d %s\n", *md.TotalPayout, md.TotalPayoutCurrency)
			}
			if md.AvgReward != nil {
				fmt.Printf("  Avg reward: %d %s\n", *md.AvgReward, md.TotalPayoutCurrency)
			}
			if md.AvgFirstResponseDays != nil {
				fmt.Printf("  Avg first response: %.2f days\n", *md.AvgFirstResponseDays)
			}
		}
		if md.ScopesCount != nil {
			fmt.Printf("\nScopes count: %d\n", *md.ScopesCount)
		}

		if len(md.QualifyingVulnerabilities) > 0 {
			fmt.Println("\nQualifying vulnerabilities:")
			for _, v := range md.QualifyingVulnerabilities {
				fmt.Printf("  - %s\n", v)
			}
		}
		if len(md.NonQualifyingVulnerabilities) > 0 {
			fmt.Println("\nNon-qualifying vulnerabilities:")
			for _, v := range md.NonQualifyingVulnerabilities {
				fmt.Printf("  - %s\n", v)
			}
		}
		if len(md.OutOfScopeSummary) > 0 {
			fmt.Println("\nOut-of-scope (summary):")
			for _, v := range md.OutOfScopeSummary {
				fmt.Printf("  - %s\n", v)
			}
		}

		if md.InScopeDescription != "" {
			fmt.Println("\nIn-scope description:")
			fmt.Println(md.InScopeDescription)
		}
		if md.AccountAccess != "" {
			fmt.Println("\nAccount access:")
			fmt.Println(md.AccountAccess)
		}
		if md.CanCreateTestAccount != nil {
			fmt.Printf("\nCan create test account: %v\n", *md.CanCreateTestAccount)
		}
		if md.UserAgent != "" {
			fmt.Printf("Suggested User-Agent: %s\n", md.UserAgent)
		}
		if md.RequestHeader != "" {
			fmt.Printf("Required request header: %s\n", md.RequestHeader)
		}
		if md.AutomatedToolingLimit != nil {
			fmt.Printf("Automated tooling limit: %d req/s\n", *md.AutomatedToolingLimit)
		}
		if md.VPNRequired != nil && *md.VPNRequired {
			fmt.Printf("VPN required: %v\n", *md.VPNRequired)
			if len(md.VNPIPs) > 0 {
				fmt.Printf("VPN IPs: %s\n", strings.Join(md.VNPIPs, ", "))
			}
		}
		if md.FAQs != "" {
			fmt.Println("\nFAQs:")
			fmt.Println(md.FAQs)
		}
		if len(md.Tags) > 0 {
			fmt.Printf("\nTags: %s\n", strings.Join(md.Tags, ", "))
		}
		if md.Rules != "" {
			fmt.Println("\nRules:")
			fmt.Println(md.Rules)
		}

		return nil
	},
}

func boolOrDash(b *bool) string {
	if b == nil {
		return "-"
	}
	if *b {
		return "yes"
	}
	return "no"
}

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(statsCmd)
	dbCmd.AddCommand(changesCmd)
	dbCmd.AddCommand(printCmd)
	dbCmd.AddCommand(findCmd)
	dbCmd.AddCommand(addCmd)
	dbCmd.AddCommand(shellCmd)
	dbCmd.AddCommand(programMetaCmd)
	addCmd.Flags().StringP("target", "t", "", "Target to add (can be comma-separated)")
	addCmd.Flags().StringP("category", "c", "wildcard", "Category of the target")
	addCmd.Flags().StringP("program-url", "u", "custom", "Program URL (default: 'custom')")
	changesCmd.Flags().Int("limit", 50, "Number of recent changes to show")
	changesCmd.Flags().String("since", "", "Show changes since: today, yesterday, 7d, 30d, 90d, 1y, or YYYY-MM-DD")
	changesCmd.Flags().String("until", "", "Show changes until: today, yesterday, 7d, 30d, 90d, 1y, or YYYY-MM-DD")
	printCmd.Flags().String("platform", "all", "Comma-separated platforms (h1,bc,it,ywh,immunefi) or 'all'")
	printCmd.Flags().String("program", "", "Filter by program handle or full URL")
	printCmd.Flags().Bool("oos", false, "Include out-of-scope elements")
	printCmd.Flags().String("since", "", "Only include entries/changes since this RFC3339 timestamp")
	printCmd.Flags().String("format", "txt", "Output format: txt|json|csv")
	printCmd.Flags().StringP("delimiter", "d", " ", "Delimiter character to use for txt output format")
	printCmd.Flags().StringP("output", "o", "tu", "Output flags. Supported: t (target), d (target description), c (category), u (program URL). Can be combined. Example: -o tdu")
	printCmd.Flags().Bool("include-ignored", false, "Include programs that are marked as ignored")
}
