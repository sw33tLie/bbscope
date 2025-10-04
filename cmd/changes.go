package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

var changesCmd = &cobra.Command{
	Use:   "changes",
	Short: "Show recent scope changes (default 50)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		dbPath, _ := cmd.Flags().GetString("dbpath")
		limit, _ := cmd.Flags().GetInt("limit")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}
		if _, err := os.Stat(dbPath); err != nil {
			return fmt.Errorf("database not found: %s", dbPath)
		}
		db, err := storage.Open(dbPath)
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
			fmt.Printf("%s  %-6s  %s  %s  %s  in_scope=%t\n", ts, c.ChangeType, c.Platform, c.ProgramURL, c.TargetNormalized, c.InScope)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(changesCmd)
	changesCmd.Flags().String("dbpath", "", "Path to SQLite DB file (default: bbscope.sqlite in CWD)")
	changesCmd.Flags().Int("limit", 50, "Number of recent changes to show")
}
