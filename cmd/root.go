package cmd

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/internal/utils"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "bbscope",
	Short: "Grab scope from HackerOne, Bugcrowd, Intigriti and YesWeHack",
	Long:  `The ultimate scope gathering tool for HackerOne, Bugcrowd, Intigriti and YesWeHack by sw33tLie`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.bbscope.yaml)")

	// Global flags
	rootCmd.PersistentFlags().StringP("proxy", "", "", "HTTP Proxy (Useful for debugging. Example: http://127.0.0.1:8080)")
	rootCmd.PersistentFlags().StringP("output", "o", "t", "Output flags. Supported: t (target), d (target description), c (category), u (program URL). Can be combined. Example: -o tdu")
	rootCmd.PersistentFlags().StringP("delimiter", "d", " ", "Delimiter character used when printing multiple data using the output flag")
	rootCmd.PersistentFlags().BoolP("bbpOnly", "b", false, "Only fetch programs offering monetary rewards")
	rootCmd.PersistentFlags().BoolP("pvtOnly", "p", false, "Only fetch data from private programs")
	rootCmd.PersistentFlags().StringP("loglevel", "l", "info", "Set log level. Available: debug, info, warn, error, fatal")
	rootCmd.PersistentFlags().BoolP("oos", "", false, "Also print out of scope items with [OOS] - Intigriti only for now")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".bbscope" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".bbscope")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		utils.Log.Debug("Found config file")
	}

	// Init log library
	levelString, _ := rootCmd.PersistentFlags().GetString("loglevel")
	utils.SetLogLevel(levelString)

	// Initialize rand for any subcommand
	rand.Seed(time.Now().Unix())
}
