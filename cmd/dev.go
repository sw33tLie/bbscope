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
	devCmd.Flags().String("dbpath", "~/bbscope-data/bbscope.sqlite", "Path to the bbscope SQLite database.")
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

	type diffEntry struct {
		Platform   string
		ProgramURL string
		Category   string
		Raw        string
		AI         []string
	}

	group := make(map[string]*diffEntry)
	order := make([]*diffEntry, 0)

	for _, entry := range entries {
		if entry.Source != "ai" {
			continue
		}
		if strings.EqualFold(entry.TargetNormalized, entry.TargetRaw) {
			continue
		}

		key := strings.ToLower(fmt.Sprintf("%s|%s|%s|%s",
			entry.Platform, entry.ProgramURL, entry.Category, entry.TargetRaw))

		target := group[key]
		if target == nil {
			target = &diffEntry{
				Platform:   entry.Platform,
				ProgramURL: entry.ProgramURL,
				Category:   entry.Category,
				Raw:        entry.TargetRaw,
				AI:         []string{},
			}
			group[key] = target
			order = append(order, target)
		}

		exists := false
		for _, val := range target.AI {
			if strings.EqualFold(val, entry.TargetNormalized) {
				exists = true
				break
			}
		}
		if !exists {
			target.AI = append(target.AI, entry.TargetNormalized)
		}
	}

	if len(order) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No AI-adjusted scope entries differ from their raw values.")
		return nil
	}

	for _, target := range order {
		suffix := ""
		if len(target.AI) > 1 {
			suffix = " (MULTIPLE)"
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"%s | %s | %s | raw: %s | ai: %s%s\n",
			target.Platform,
			target.ProgramURL,
			target.Category,
			target.Raw,
			strings.Join(target.AI, " "),
			suffix,
		)
	}

	return nil
}
