package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development helpers",
	RunE:  runDevCmd,
}

func init() {
	devCmd.Flags().String("dbpath", "bbscope.sqlite", "Path to the bbscope SQLite database.")
	rootCmd.AddCommand(devCmd)
}

func runDevCmd(cmd *cobra.Command, args []string) error {
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

	db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	entries, err := db.ListEntries(context.Background(), storage.ListOptions{
		IncludeOOS:     true,
		IncludeIgnored: true,
	})
	if err != nil {
		return err
	}

	var diffCount int
	for _, entry := range entries {
		if entry.Source != "ai" {
			continue
		}
		if strings.EqualFold(entry.TargetNormalized, entry.TargetRaw) {
			continue
		}

		diffCount++
		fmt.Fprintf(cmd.OutOrStdout(),
			"%s | %s | %s | raw: %s | ai: %s\n",
			entry.Platform,
			entry.ProgramURL,
			entry.Category,
			entry.TargetRaw,
			entry.TargetNormalized,
		)
	}

	if diffCount == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No AI-adjusted scope entries differ from their raw values.")
	}

	return nil
}
