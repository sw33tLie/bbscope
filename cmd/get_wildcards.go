package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
	"github.com/sw33tLie/bbscope/v2/pkg/targets"
)

var getWildcardsCmd = &cobra.Command{
	Use:   "wildcards",
	Short: "Get all targets that are wildcards",
	RunE:  runGetWildcardsCmd,
}

func init() {
	getWildcardsCmd.Flags().BoolP("aggressive", "a", false, "Extract root domains from all URL targets in addition to wildcards.")
	getWildcardsCmd.Flags().String("platform", "all", "Limit results to a specific platform (e.g. h1, bugcrowd, intigriti).")
	getWildcardsCmd.Flags().StringP("output", "o", "t", "Output flags. Supported: t (target), u (program URL). Example: -o tu")
	getWildcardsCmd.Flags().StringP("delimiter", "d", " ", "Delimiter to use between output fields.")
	getCmd.AddCommand(getWildcardsCmd)
}

func runGetWildcardsCmd(cmd *cobra.Command, args []string) error {
	dbURL, err := GetDBConnectionString()
	if err != nil {
		return err
	}

	db, err := storage.Open(dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	platformFilter, _ := cmd.Flags().GetString("platform")
	listOpts := storage.ListOptions{
		Platform:   platformFilter,
		IncludeOOS: true,
	}

	entries, err := db.ListEntries(context.Background(), listOpts)
	if err != nil {
		return err
	}

	aggressive, _ := cmd.Flags().GetBool("aggressive")
	outputFlags, _ := cmd.Flags().GetString("output")
	if outputFlags == "" {
		outputFlags = "t"
	}
	for _, flag := range outputFlags {
		if flag != 't' && flag != 'u' {
			return fmt.Errorf("invalid output flag '%c'. Supported flags: t, u", flag)
		}
	}
	delimiter, _ := cmd.Flags().GetString("delimiter")

	opts := targets.WildcardOptions{
		Aggressive: aggressive,
	}

	results := targets.CollectWildcardsSorted(entries, opts)
	includeProgram := strings.ContainsRune(outputFlags, 'u')

	for _, result := range results {
		if includeProgram {
			for _, programURL := range result.ProgramURLs {
				line := formatWildcardLine(result.Domain, programURL, outputFlags, delimiter)
				if line != "" {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			}
			continue
		}

		line := formatWildcardLine(result.Domain, "", outputFlags, delimiter)
		if line != "" {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}

	return nil
}

func formatWildcardLine(domain, programURL, outputFlags, delimiter string) string {
	var builder strings.Builder
	for _, flag := range outputFlags {
		switch flag {
		case 't':
			builder.WriteString(domain)
			builder.WriteString(delimiter)
		case 'u':
			builder.WriteString(programURL)
			builder.WriteString(delimiter)
		}
	}
	line := builder.String()
	line = strings.TrimSuffix(line, delimiter)
	return line
}
