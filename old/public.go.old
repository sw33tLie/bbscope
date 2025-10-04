/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/scope"
)

var validPlatforms = []string{"bc", "h1", "it", "ywh"}

// publicCmd represents the public command
var publicCmd = &cobra.Command{
	Use:   "public [platforms]",
	Short: "Instantly download public scope from bbscope.com",
	Long: `Download public scope from bbscope.com for specified platforms.
Supported platforms: bc (Bugcrowd), h1 (HackerOne), it (Intigriti), ywh (YesWeHack).
Multiple platforms can be specified comma-separated (e.g., "bc,h1").
Use "all" to fetch from all platforms.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		includeOOS, _ := rootCmd.PersistentFlags().GetBool("oos")

		platformList := strings.Split(args[0], ",")
		if len(platformList) == 1 && platformList[0] == "all" {
			platformList = validPlatforms
		}

		for _, platform := range platformList {
			platform = strings.TrimSpace(platform)
			if !isValidPlatform(platform) {
				fmt.Printf("Invalid platform: %s\n", platform)
				continue
			}

			url := fmt.Sprintf("https://bbscope.com/static/scope/latest-%s.json", platform)
			resp, err := http.Get(url)
			if err != nil {
				fmt.Printf("Error fetching %s: %v\n", platform, err)
				continue
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("Error reading response for %s: %v\n", platform, err)
				continue
			}

			var programs []scope.ProgramData
			if err := json.Unmarshal(body, &programs); err != nil {
				fmt.Printf("Error parsing JSON for %s: %v\n", platform, err)
				continue
			}

			for _, program := range programs {
				scope.PrintProgramScope(program, outputFlags, delimiterCharacter, includeOOS)
			}
		}
	},
}

func isValidPlatform(platform string) bool {
	for _, validPlatform := range validPlatforms {
		if platform == validPlatform {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(publicCmd)
}
