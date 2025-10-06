package scope

import (
	"fmt"
	"log"
	"strings"
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
	"ios":        {"ios", "apple_store_app_id", "other_ipa", "testflight", "mobile-application-ios", "apple-store"},
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
