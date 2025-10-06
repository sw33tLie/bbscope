package scope

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
)

type ScopeElement struct {
	Target      string
	Description string
	Category    string
}

type ProgramData struct {
	Url        string
	InScope    []ScopeElement
	OutOfScope []ScopeElement
}

func PrintProgramScope(programScope ProgramData, outputFlags string, delimiter string, includeOOS bool) {
	printScope := func(scope []ScopeElement, prefix string) {
		for _, scopeElement := range scope {
			line := createLine(scopeElement, programScope.Url, outputFlags, delimiter)
			if len(line) > 0 {
				fmt.Println(prefix + line)
			}
		}
	}

	printScope(programScope.InScope, "")
	if includeOOS {
		printScope(programScope.OutOfScope, "[OOS] ")
	}
}

func createLine(scopeElement ScopeElement, url, outputFlags, delimiter string) string {
	var line string
	// Unify category before printing
	unifiedCategory := NormalizeCategory(scopeElement.Category)

	for _, f := range outputFlags {
		switch f {
		case 't':
			line += scopeElement.Target + delimiter
		case 'd':
			line += scopeElement.Description + delimiter
		case 'c':
			line += unifiedCategory + delimiter
		case 'u':
			line += url + delimiter
		default:
			log.Fatal("Invalid print flag")
		}
	}
	return strings.TrimSuffix(line, delimiter)
}

// unificationMap is the source of truth for category normalization.
// It groups raw, platform-specific category strings under a unified category name.
var unificationMap = map[string][]string{
	"wildcard":   {"wildcard"},
	"url":        {"url", "website", "web", "web-application", "api", "ip_address", "ip-address"},
	"cidr":       {"cidr", "iprange"},
	"android":    {"android", "google_play_app_id", "other_apk", "mobile-application-android", "mobile-application"},
	"ios":        {"ios", "apple", "apple_store_app_id", "other_ipa", "testflight", "mobile-application-ios", "apple-store"},
	"ai":         {"ai_model"},
	"hardware":   {"hardware", "device", "iot"},
	"blockchain": {"smart_contract"},
	"binary":     {"windows_app_store_app_id", "downloadable_executables"},
	"code":       {"source_code"},
	"other":      {"other", "aws_cloud_config", "application", "network"},
}

// categoryMap is a reverse map generated from unificationMap for efficient lookups.
var categoryMap map[string]string

func init() {
	categoryMap = make(map[string]string)
	for unified, raws := range unificationMap {
		for _, raw := range raws {
			categoryMap[raw] = unified
		}
	}
}

func NormalizeCategory(category string) string {

	// Normalize category string for matching
	catLower := strings.ToLower(category)

	if unified, ok := categoryMap[catLower]; ok {
		return unified
	}

	// For anything else, just format it nicely.
	return strings.ReplaceAll(catLower, "_", " ")
}

func GetAllStringsForCategories(input string) []string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "all" || input == "" {
		return nil // nil means don't filter
	}

	// Use a map to handle duplicates automatically
	finalCategoriesSet := make(map[string]bool)

	// Split comma-separated values
	rawCategories := strings.Split(input, ",")

	for _, rawCategory := range rawCategories {
		categoryKey := strings.TrimSpace(rawCategory)

		// Look up in the unificationMap
		platformSpecificStrings, ok := unificationMap[categoryKey]
		if !ok {
			utils.Log.Warnf("Invalid category '%s' selected, it will be ignored.", categoryKey)
			continue // Skip invalid category
		}

		for _, s := range platformSpecificStrings {
			finalCategoriesSet[s] = true
		}
	}

	if len(finalCategoriesSet) == 0 {
		validCategories := make([]string, 0, len(unificationMap))
		for k := range unificationMap {
			validCategories = append(validCategories, k)
		}
		sort.Strings(validCategories)
		utils.Log.Warnf("No valid categories provided, defaulting to all. Available categories: %s", strings.Join(validCategories, ", "))
		return nil
	}

	// Convert map keys to a slice
	result := make([]string, 0, len(finalCategoriesSet))
	for category := range finalCategoriesSet {
		result = append(result, category)
	}

	return result
}
