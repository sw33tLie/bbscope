package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	devplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/dev"
)

// poll it: Intigriti
var pollDevCmd = &cobra.Command{
	Use:   "dev",
	Short: "Poll Intigriti programs",
	RunE: func(cmd *cobra.Command, _ []string) error {

		poller := devplatform.NewPoller()
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{}); err != nil {
			return err
		}

		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollDevCmd)
}
