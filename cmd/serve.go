package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/website/pkg/core"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the bbscope.com web server",
	RunE: func(cmd *cobra.Command, args []string) error {
		devMode, _ := cmd.Flags().GetBool("dev")
		dbURL, err := GetDBConnectionString()
		if err != nil {
			return err
		}
		pollInterval, _ := cmd.Flags().GetInt("poll-interval")
		listenAddr, _ := cmd.Flags().GetString("listen")
		domain, _ := cmd.Flags().GetString("domain")

		return core.Run(core.ServerConfig{
			DevMode:      devMode,
			DBUrl:        dbURL,
			PollInterval: pollInterval,
			ListenAddr:   listenAddr,
			Domain:       domain,
		})
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().BoolP("dev", "d", false, "Enable development mode (HTTP on localhost:7000)")
	serveCmd.Flags().Int("poll-interval", 6, "Hours between polling cycles (0 to disable)")
	serveCmd.Flags().String("listen", ":8080", "HTTP listen address")
	serveCmd.Flags().String("domain", "bbscope.com", "Domain name for sitemap/robots.txt")
}
