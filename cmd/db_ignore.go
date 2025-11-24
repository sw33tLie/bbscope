package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// ignoreCmd represents the ignore command
var ignoreCmd = &cobra.Command{
	Use:   "ignore",
	Short: "Mark a program as ignored to skip it during polling",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern, err := resolveProgramPattern(cmd)
		if err != nil {
			return err
		}
		return setIgnoreStatus(cmd.Context(), pattern, true)
	},
}

// unignoreCmd represents the unignore command
var unignoreCmd = &cobra.Command{
	Use:   "unignore",
	Short: "Unmark a program as ignored",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		pattern, err := resolveProgramPattern(cmd)
		if err != nil {
			return err
		}
		return setIgnoreStatus(cmd.Context(), pattern, false)
	},
}

func resolveProgramPattern(cmd *cobra.Command) (string, error) {
	flagValue, _ := cmd.Flags().GetString("program-url")
	if flagValue == "" {
		return "", fmt.Errorf("please provide a program URL via --program-url")
	}
	return flagValue, nil
}

func setIgnoreStatus(ctx context.Context, pattern string, ignored bool) error {
	dbPathRaw, _ := dbCmd.PersistentFlags().GetString("dbpath")
	dbPath, err := expandPath(dbPathRaw)
	if err != nil {
		return err
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
	ignoreCmd.Flags().StringP("program-url", "u", "", "Program URL or pattern to ignore")
	unignoreCmd.Flags().StringP("program-url", "u", "", "Program URL or pattern to unignore")
}
