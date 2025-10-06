package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"text/tabwriter"

	"github.com/spf13/cobra"
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

func init() {
	rootCmd.AddCommand(dbCmd)
	dbCmd.AddCommand(shellCmd)
	dbCmd.AddCommand(statsCmd)
	dbCmd.PersistentFlags().StringVar(&dbPath, "dbpath", "bbscope.sqlite", "Path to SQLite DB file")
}
