package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Extract specific scope types from the database based on format",
}

var getURLsCmd = &cobra.Command{
	Use:   "urls",
	Short: "Get all targets that are URLs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrintTargets("urls", false)
	},
}

var getWildcardsCmd = &cobra.Command{
	Use:   "wildcards",
	Short: "Get all targets that are wildcards",
	RunE:  runGetWildcardsCmd,
}

var getDomainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "Get all targets that are domains (including wildcards)",
	RunE: func(cmd *cobra.Command, args []string) error {
		aggressive, _ := cmd.Flags().GetBool("aggressive")
		return getAndPrintTargets("domains", aggressive)
	},
}

func getAndPrintTargets(targetType string, aggressive bool) error {
	dbPath, _ := getCmd.PersistentFlags().GetString("dbpath")
	if dbPath == "" {
		dbPath = "bbscope.sqlite"
	}
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database not found: %s", dbPath)
		}
		return err
	}

	dbTimeout, _ := getCmd.PersistentFlags().GetInt("db-timeout")
	db, err := storage.Open(dbPath, dbTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	entries, err := db.ListEntries(context.Background(), storage.ListOptions{
		// For now, we get all entries and filter locally.
		// This could be optimized with more specific DB queries in the future.
	})
	if err != nil {
		return err
	}

	for _, e := range entries {
		target := e.TargetNormalized
		if aggressive {
			target = storage.AggressiveTransform(target)
		}

		switch targetType {
		case "urls":
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				fmt.Println(target)
			}
		case "wildcards":
			if strings.HasPrefix(target, "*.") {
				fmt.Println(target)
			}
		case "domains":
			// A simple check for a dot is sufficient for now, as NormalizeTarget cleans things up.
			// URLs will have their hosts extracted by the aggressive transform if used.
			if strings.Contains(target, ".") && !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
				fmt.Println(target)
			}
		}
	}

	return nil
}

func init() {
	dbCmd.AddCommand(getCmd)
	getCmd.AddCommand(getURLsCmd)
	getCmd.AddCommand(getWildcardsCmd)
	getCmd.AddCommand(getDomainsCmd)
	getWildcardsCmd.Flags().BoolP("aggressive", "a", false, "Extract root domains from all URL targets in addition to wildcards.")
	getDomainsCmd.Flags().BoolP("aggressive", "a", false, "Apply aggressive scope transformation")
}

func runGetWildcardsCmd(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Parent().Flags().GetString("dbpath")
	if dbPath == "" {
		dbPath = "bbscope.sqlite"
	}
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database not found: %s", dbPath)
		}
		return err
	}

	dbTimeout, _ := cmd.Parent().Flags().GetInt("db-timeout")
	db, err := storage.Open(dbPath, dbTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	// Fetch all entries, including out-of-scope to check against them
	entries, err := db.ListEntries(context.Background(), storage.ListOptions{IncludeOOS: true})
	if err != nil {
		return err
	}

	aggressive, _ := cmd.Flags().GetBool("aggressive")
	uniqueDomains := make(map[string]struct{})

	// Pre-process and store out-of-scope targets per program for quick lookup
	outOfScopeByProgram := make(map[string]map[string]struct{})
	for _, e := range entries {
		if !e.InScope {
			if _, ok := outOfScopeByProgram[e.ProgramURL]; !ok {
				outOfScopeByProgram[e.ProgramURL] = make(map[string]struct{})
			}
			normalizedOOS := normalizeForSubdomainTools(e.TargetNormalized)
			if normalizedOOS != "" {
				outOfScopeByProgram[e.ProgramURL][normalizedOOS] = struct{}{}
			}
		}
	}

Loop:
	for _, e := range entries {
		if !e.InScope {
			utils.Log.Debug("[skip-oos] ", e.TargetNormalized)
			continue
		}

		// If a target still contains spaces after initial normalization, it's not a valid domain for our purposes.
		if strings.Contains(e.TargetNormalized, " ") {
			utils.Log.Debug("[skip-space] ", e.TargetNormalized)
			continue
		}

		// Use the normalization function to get a clean hostname for evaluation
		cleanHost := normalizeForSubdomainTools(e.TargetNormalized)
		if cleanHost == "" {
			continue
		}

		// Exclude platform-as-a-service domains that are too broad to be useful.
		// Find more manually with go run *.go db get wildcards -a | egrep '\.(.*\.){1,}' | sort -u
		blacklistedSuffixes := []string{
			"amazonaws.com",
			"amazoncognito.com",
			"azurewebsites.net",
			"cloudfront.net",
			"herokuapp.com",
			"appspot.com",
			"myshopify.com",
			"github.io",
			"netlify.app",
			"aivencloud.com",
			"pstmn.io",
			"googleapis.com",
			"amazon.com.be",
			"vercel.app",
			"webhosting.be",
			"adobeaemcloud.com",
			"azurefd.net",
			"windows.net",
			"strapiapp.com",
			"forgeblocks.com",
		}

		for _, suffix := range blacklistedSuffixes {
			if strings.HasSuffix(cleanHost, "."+suffix) || cleanHost == suffix {
				continue Loop
			}
		}

		var finalDomain string

		// 1. Handle explicit wildcards first.
		isExplicitWildcard := e.Category == "wildcard" || strings.Contains(e.TargetNormalized, "*")

		if isExplicitWildcard {
			normalized := normalizeForSubdomainTools(e.TargetNormalized)
			if root, ok := storage.ExtractRootDomain(normalized); ok {
				finalDomain = root
			} else {
				utils.Log.Debug("[skip] ", e.TargetNormalized)
			}
		} else if aggressive {
			// 2. Handle other types only in aggressive mode, applying extensive filtering.
			category := strings.ToLower(e.Category)
			target := strings.ToLower(e.TargetNormalized)

			nonDomainCategories := map[string]struct{}{
				"android":    {},
				"ios":        {},
				"binary":     {},
				"code":       {},
				"ai":         {},
				"hardware":   {},
				"blockchain": {},
			}
			if _, found := nonDomainCategories[category]; found {
				continue
			}

			if strings.HasPrefix(target, "com.") ||
				strings.Contains(target, "apps.apple.com") ||
				strings.HasSuffix(target, ".apk") ||
				strings.HasSuffix(target, ".ipa") ||
				strings.HasSuffix(target, ".ios") ||
				strings.HasSuffix(target, ".exe") {
				continue
			}

			if utils.IsCIDR(target) || utils.IsIP(target) || utils.IsIPRange(target) {
				continue
			}

			// First, apply the same normalization to clean up the target string
			normalized := normalizeForSubdomainTools(target)
			if rootDomain, ok := storage.ExtractRootDomain(normalized); ok {
				finalDomain = rootDomain
			}
		}

		// 3. Add the processed domain to the set if it's not empty and not out-of-scope.
		if finalDomain != "" {
			if programOOS, programExists := outOfScopeByProgram[e.ProgramURL]; programExists {
				if _, isOOS := programOOS[finalDomain]; isOOS {
					continue // It's out-of-scope, so skip it
				}
			}
			uniqueDomains[finalDomain] = struct{}{}
		}
	}

	for domain := range uniqueDomains {
		fmt.Println(domain)
	}

	return nil
}

// normalizeForSubdomainTools takes a raw scope string and cleans it up for subdomain enumeration tools.
// It returns just the domain part (e.g., "http://*.example.com/path" -> "example.com").
// This function is an adaptation of your original normalizeWildcard function to clean up
// various non-standard wildcard formats found in bug bounty scopes.
func normalizeForSubdomainTools(scope string) string {
	var processingStr string
	// First, try to parse it as a URL to robustly get the host
	if u, err := url.Parse(scope); err == nil && u.Host != "" {
		processingStr = u.Host
	} else {
		// Not a valid URL, start with the raw string
		processingStr = scope
	}

	// Remove path if present (might still be there if not a valid URL)
	processingStr = strings.Split(processingStr, "/")[0]
	// Also remove port
	processingStr = strings.Split(processingStr, ":")[0]

	if strings.HasSuffix(processingStr, ".*") {
		processingStr = strings.TrimSuffix(processingStr, ".*") + ".com"
	}

	if strings.HasSuffix(processingStr, ".<tld>") {
		processingStr = strings.TrimSuffix(processingStr, ".<tld>") + ".com"
	}

	// Replace "*" with empty string for subdomain tool input
	processingStr = strings.ReplaceAll(processingStr, "*", "")

	// Remove any left ,
	processingStr = strings.ReplaceAll(processingStr, ",", ".")

	// Trim starting .
	processingStr = strings.TrimPrefix(processingStr, ".")

	// Remove ( )
	processingStr = strings.ReplaceAll(processingStr, "(", "")
	processingStr = strings.ReplaceAll(processingStr, ")", "")

	// Handle patterns like console-backend.truelayer[-sandbox].com
	processingStr = regexp.MustCompile(`\[.*?\]`).ReplaceAllString(processingStr, "")

	// Final trim for any leftover whitespace or dots
	processingStr = strings.Trim(processingStr, ". ")

	return processingStr
}
