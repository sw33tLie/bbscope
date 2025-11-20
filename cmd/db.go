package cmd

import (
	"context"
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

var dbPath string

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Interact with the bbscope database",
}

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Start an interactive shell to the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, _ := cmd.Parent().Flags().GetString("dbpath")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			return fmt.Errorf("database file not found: %s", dbPath)
		}

		// Check if sqlite3 is in PATH
		sqlitePath, err := exec.LookPath("sqlite3")
		if err != nil {
			return fmt.Errorf("sqlite3 command not found in your PATH. Please install it to use the db shell")
		}

		// Print schema first
		fmt.Println("--> Database schema:")
		schemaCmd := exec.Command(sqlitePath, dbPath, ".schema")
		schemaCmd.Stdout = os.Stdout
		schemaCmd.Stderr = os.Stderr
		if err := schemaCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't retrieve schema: %v\n", err)
		}
		fmt.Println("\n--> Starting interactive shell... (Ctrl+D to exit)")

		c := exec.Command(sqlitePath, dbPath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		return c.Run()
	},
}

// statsCmd represents the stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Prints statistics about the programs and assets in the database.",
	Long:  "Prints statistics about the programs and assets in the database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, _ := cmd.Parent().Flags().GetString("dbpath")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}

		db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("database file not found: %s", dbPath)
			}
			return err
		}
		defer db.Close()

		stats, err := db.GetStats(context.Background())
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
	RunE: func(cmd *cobra.Command, _ []string) error {
		dbPath, _ := cmd.Parent().Flags().GetString("dbpath")
		limit, _ := cmd.Flags().GetInt("limit")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}
		if _, err := os.Stat(dbPath); err != nil {
			return fmt.Errorf("database not found: %s", dbPath)
		}
		db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
		if err != nil {
			return err
		}
		defer db.Close()
		changes, err := db.ListRecentChanges(context.Background(), limit)
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
		dbPath, _ := cmd.Parent().Flags().GetString("dbpath")

		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}
		if _, err := os.Stat(dbPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("database not found: %s", dbPath)
			}
			return err
		}

		db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
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
			// Minimal JSON; keep it simple for now
			fmt.Print("[")
			for i, e := range filtered {
				if i > 0 {
					fmt.Print(",")
				}
				fmt.Printf("{\"program_url\":\"%s\",\"platform\":\"%s\",\"handle\":\"%s\",\"target\":\"%s\",\"category\":\"%s\",\"description\":\"%s\",\"in_scope\":%t}", e.ProgramURL, e.Platform, e.Handle, e.TargetNormalized, e.Category, strings.ReplaceAll(e.Description, "\"", "\\\""), e.InScope)
			}
			fmt.Println("]")
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
		dbPath, _ := cmd.Parent().Flags().GetString("dbpath")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}

		db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
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

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a custom target to the database",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("target")
		category, _ := cmd.Flags().GetString("category")
		programURL, _ := cmd.Flags().GetString("program-url")
		dbPath, _ := cmd.Parent().Flags().GetString("dbpath")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}

		if target == "" {
			return errors.New("target flag is required")
		}

		db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
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

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(shellCmd)
	dbCmd.AddCommand(statsCmd)
	dbCmd.AddCommand(changesCmd)
	dbCmd.AddCommand(printCmd)
	dbCmd.AddCommand(findCmd)
	dbCmd.AddCommand(addCmd)
	addCmd.Flags().StringP("target", "t", "", "Target to add (can be comma-separated)")
	addCmd.Flags().StringP("category", "c", "wildcard", "Category of the target")
	addCmd.Flags().StringP("program-url", "u", "custom", "Program URL (default: 'custom')")
	changesCmd.Flags().Int("limit", 50, "Number of recent changes to show")
	printCmd.Flags().String("platform", "all", "Comma-separated platforms (h1,bc,it,ywh,immunefi) or 'all'")
	printCmd.Flags().String("program", "", "Filter by program handle or full URL")
	printCmd.Flags().Bool("oos", false, "Include out-of-scope elements")
	printCmd.Flags().String("since", "", "Only include entries/changes since this RFC3339 timestamp")
	printCmd.Flags().String("format", "txt", "Output format: txt|json|csv")
	printCmd.Flags().StringP("delimiter", "d", " ", "Delimiter character to use for txt output format")
	printCmd.Flags().StringP("output", "o", "tu", "Output flags. Supported: t (target), d (target description), c (category), u (program URL). Can be combined. Example: -o tdu")
	printCmd.Flags().Bool("include-ignored", false, "Include programs that are marked as ignored")
	dbCmd.PersistentFlags().StringVar(&dbPath, "dbpath", "bbscope.sqlite", "Path to SQLite DB file")
}
