package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/internal/server"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// webCmd represents the web command
var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the bbscope web interface",
	Long:  `Start a web server to view and manage your scope.`,
	Run: func(cmd *cobra.Command, args []string) {
		dbURL, err := GetDBConnectionString()
		if err != nil {
			log.Fatalf("Failed to get DB config: %v", err)
		}

		db, err := storage.Open(dbURL)
		if err != nil {
			log.Fatalf("Failed to open DB: %v", err)
		}
		defer db.Close()

		// Auth
		user, _ := cmd.Flags().GetString("username")
		pass, _ := cmd.Flags().GetString("password")
		addr, _ := cmd.Flags().GetString("bind")

		srv := server.New(db, user, pass)
		if err := srv.Start(addr); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(webCmd)

	webCmd.Flags().StringP("bind", "b", ":9999", "Address to bind the server to")
	webCmd.Flags().StringP("username", "u", "", "Username for basic auth (optional)")
	webCmd.Flags().StringP("password", "p", "", "Password for basic auth (optional)")
}
