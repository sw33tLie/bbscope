package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	implatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/immunefi"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll immunefi: shorthand for Immunefi
var pollImmunefiCmd = &cobra.Command{
	Use:   "immunefi",
	Short: "Poll Immunefi programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}
		poller := &implatform.Poller{}
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollImmunefiCmd)
}
