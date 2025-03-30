package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/platforms/intigriti"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// itCmd represents the it command
var itCmd = &cobra.Command{
	Use:   "it",
	Short: "Intigriti",
	Long:  "Gathers data from Intigriti (https://intigriti.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		config, _ := cmd.Flags().GetString("config")

		// Check if the config file exists and is a valid YAML file
		if config != "" {
			if _, err := os.Stat(config); err == nil {
				viper.SetConfigFile(config)
				if err := viper.ReadInConfig(); err != nil {
					utils.Log.Fatalf("Error reading config file: %v", err)
				}
				utils.Log.Info("Config file loaded successfully")

				// Check if the config file contains an "intigriti" section
				if viper.IsSet("intigriti") {
					intigritiConfig := viper.Sub("intigriti")
					if intigritiConfig.IsSet("token") {
						cmd.Flags().Set("token", intigritiConfig.GetString("token"))
					}
					if intigritiConfig.IsSet("categories") {
						cmd.Flags().Set("categories", intigritiConfig.GetString("categories"))
					}
				} else {
					utils.Log.Fatalf("Config file does not contain an 'intigriti' section")
				}
			} else {
				utils.Log.Fatalf("Config file not found: %v", err)
			}
		}

		token, _ := cmd.Flags().GetString("token")
		categories, _ := cmd.Flags().GetString("categories")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		includeOOS, _ := rootCmd.PersistentFlags().GetBool("oos")

		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		intigriti.GetAllProgramsScope(token, bbpOnly, pvtOnly, categories, outputFlags, delimiterCharacter, includeOOS, true)
	},
}

func init() {
	rootCmd.AddCommand(itCmd)
	itCmd.Flags().StringP("token", "t", "", "Intigriti API token")
	itCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, device, other, wildcard)")
}
