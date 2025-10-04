package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	h1platform "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll h1: shorthand for --platform h1 with platform-specific flags
var pollH1Cmd = &cobra.Command{
	Use:   "h1",
	Short: "Poll HackerOne programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		user, _ := cmd.Flags().GetString("user")
		token, _ := cmd.Flags().GetString("token")
		if user == "" || token == "" {
			return cmd.Usage()
		}

		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}
		poller := h1platform.NewPoller(user, token)
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollH1Cmd)
	pollH1Cmd.Flags().String("user", "", "HackerOne username")
	pollH1Cmd.Flags().String("token", "", "HackerOne API token")
	// Reuse common flags from parent via cobra's flag inheritance
}
