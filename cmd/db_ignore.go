package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// ignoreCmd represents the ignore command
var ignoreCmd = &cobra.Command{
	Use:   "ignore [program-url-pattern]",
	Short: "Mark a program as ignored to skip it during polling",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setIgnoreStatus(cmd.Context(), args[0], true)
	},
}

// unignoreCmd represents the unignore command
var unignoreCmd = &cobra.Command{
	Use:   "unignore [program-url-pattern]",
	Short: "Unmark a program as ignored",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setIgnoreStatus(cmd.Context(), args[0], false)
	},
}

func setIgnoreStatus(ctx context.Context, pattern string, ignored bool) error {
	dbPath, _ := dbCmd.PersistentFlags().GetString("dbpath")
	if dbPath == "" {
		dbPath = "bbscope.sqlite"
	}

	db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.SetProgramIgnoredStatus(ctx, pattern, ignored); err != nil {
		return err
	}

	action := "Ignored"
	if !ignored {
		action = "Unignored"
	}
	fmt.Printf("✅ Successfully %s programs matching: %s\n", action, pattern)
	return nil
}

func init() {
	dbCmd.AddCommand(ignoreCmd)
	dbCmd.AddCommand(unignoreCmd)
}
