package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	h1platform "github.com/sw33tLie/bbscope/v2/pkg/platforms/hackerone"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll h1: shorthand for --platform h1 with platform-specific flags
var pollH1Cmd = &cobra.Command{
	Use:   "h1",
	Short: "Poll HackerOne programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		user := viper.GetString("hackerone.username")
		token := viper.GetString("hackerone.token")
		if user == "" || token == "" {
			utils.Log.Error("hackerone requires a username and token")
			return nil
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
	pollH1Cmd.Flags().StringP("user", "u", "", "HackerOne username")
	pollH1Cmd.Flags().StringP("token", "t", "", "HackerOne API token")
	viper.BindPFlag("hackerone.username", pollH1Cmd.Flags().Lookup("user"))
	viper.BindPFlag("hackerone.token", pollH1Cmd.Flags().Lookup("token"))
	// Reuse common flags from parent via cobra's flag inheritance
}
