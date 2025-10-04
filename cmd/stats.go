package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// statsCmd represents the stats command
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Prints statistics about the programs and assets in the database.",
	Long:  "Prints statistics about the programs and assets in the database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, _ := cmd.Flags().GetString("dbpath")
		if dbPath == "" {
			dbPath = "bbscope.sqlite"
		}

		db, err := storage.Open(dbPath)
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
		fmt.Fprintln(w, "PLATFORM\tPROGRAMS\tTARGETS\t")

		var totalPrograms, totalTargets int
		for _, s := range stats {
			fmt.Fprintf(w, "%s\t%d\t%d\t\n", s.Platform, s.ProgramCount, s.TargetCount)
			totalPrograms += s.ProgramCount
			totalTargets += s.TargetCount
		}

		fmt.Fprintln(w, " \t \t \t")
		fmt.Fprintf(w, "TOTAL\t%d\t%d\t\n", totalPrograms, totalTargets)

		w.Flush()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
