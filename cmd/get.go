package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/scope"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// getCmd implements: bbscope get [type]
// type: all | urls | wildcards | apis | mobile (initial set)
// Flags:
//
//	--platform string   Comma-separated platforms or "all"
//	--program  string   Filter by program
//	--include-oos       Include out-of-scope in output
//	--since string      Filter changes/targets since timestamp (optional)
//	--format string     Output format: txt|json|csv
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get scope data from the database",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		typ := "all"
		if len(args) == 1 {
			typ = strings.ToLower(args[0])
		}
		platform, _ := cmd.Flags().GetString("platform")
		program, _ := cmd.Flags().GetString("program")
		includeOOS, _ := cmd.Flags().GetBool("include-oos")
		sinceStr, _ := cmd.Flags().GetString("since")
		format, _ := cmd.Flags().GetString("format")
		dbPath, _ := cmd.Flags().GetString("dbpath")

		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}
		if _, err := os.Stat(dbPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("database not found: %s", dbPath)
			}
			return err
		}

		db, err := storage.Open(dbPath)
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

		entries, err := db.ListEntries(context.Background(), storage.ListOptions{Platform: platform, ProgramFilter: program, Since: since, IncludeOOS: includeOOS})
		if err != nil {
			return err
		}

		// Basic type filtering for urls/wildcards/apis/mobile
		filtered := entries[:0]
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

		switch format {
		case "txt":
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
					} else if includeOOS {
						pd.OutOfScope = append(pd.OutOfScope, se)
					}
				}
				scope.PrintProgramScope(pd, "tu", " ", includeOOS)
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

func init() {
	rootCmd.AddCommand(getCmd)

	getCmd.Flags().String("platform", "all", "Comma-separated platforms (h1,bc,it,ywh,immunefi) or 'all'")
	getCmd.Flags().String("program", "", "Filter by program handle or full URL")
	getCmd.Flags().Bool("include-oos", false, "Include out-of-scope elements")
	getCmd.Flags().String("since", "", "Only include entries/changes since this RFC3339 timestamp")
	getCmd.Flags().String("format", "txt", "Output format: txt|json|csv")
	getCmd.Flags().String("dbpath", "", "Path to SQLite DB file (default: bbscope.sqlite in CWD)")
}
