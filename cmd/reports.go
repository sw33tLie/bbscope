package cmd

import (
	"github.com/spf13/cobra"
)

var reportsCmd = &cobra.Command{
	Use:   "reports",
	Short: "Download bug bounty reports as Markdown files",
}

func init() {
	rootCmd.AddCommand(reportsCmd)

	reportsCmd.PersistentFlags().String("output-dir", "", "Output directory for downloaded reports (required)")
	reportsCmd.PersistentFlags().StringSlice("program", nil, "Filter by program handle(s)")
	reportsCmd.PersistentFlags().StringSlice("state", nil, "Filter by report state(s) (e.g. resolved,triaged)")
	reportsCmd.PersistentFlags().StringSlice("severity", nil, "Filter by severity (e.g. high,critical)")
	reportsCmd.PersistentFlags().Bool("dry-run", false, "List reports without downloading")
	reportsCmd.PersistentFlags().Bool("overwrite", false, "Overwrite existing report files")
}
