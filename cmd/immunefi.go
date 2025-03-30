package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/platforms/immunefi"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// immunefiCmd represents the immunefi command
var immunefiCmd = &cobra.Command{
	Use:   "immunefi",
	Short: "Immunefi",
	Long:  "Gathers data from Immunefi (https://immunefi.com/explore)",
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

				// Check if the config file contains an "immunefi" section
				if viper.IsSet("immunefi") {
					immunefiConfig := viper.Sub("immunefi")
					if immunefiConfig.IsSet("categories") {
						cmd.Flags().Set("categories", immunefiConfig.GetString("categories"))
					}
					if immunefiConfig.IsSet("concurrency") {
						cmd.Flags().Set("concurrency", immunefiConfig.GetString("concurrency"))
					}
				} else {
					utils.Log.Fatalf("Config file does not contain an 'immunefi' section")
				}
			} else {
				utils.Log.Fatalf("Config file not found: %v", err)
			}
		}

		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		categories, _ := cmd.Flags().GetString("categories")
		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		concurrency, _ := cmd.Flags().GetInt("concurrency")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		immunefi.PrintAllScope(categories, outputFlags, delimiterCharacter, concurrency)
	},
}

func init() {
	rootCmd.AddCommand(immunefiCmd)
	immunefiCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, web, contracts)")
	immunefiCmd.Flags().IntP("concurrency", "", 5, "Concurrency threshold")
}
