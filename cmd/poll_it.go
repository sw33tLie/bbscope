package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	itplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/intigriti"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll it: shorthand for Intigriti
var pollItCmd = &cobra.Command{
	Use:   "it",
	Short: "Poll Intigriti programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		token, _ := cmd.Flags().GetString("token")
		if token == "" {
			return cmd.Usage()
		}

		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}
		poller := itplatform.NewPoller(token)
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollItCmd)
	pollItCmd.Flags().String("token", "", "Intigriti token")
	pollItCmd.Flags().String("categories", "all", "Scope categories: all,url,cidr,mobile,android,apple,device,other,wildcard")
}
