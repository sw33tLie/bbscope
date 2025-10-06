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
	unifiedCategory := CategoryUnifier(scopeElement.Category, scopeElement.Target)

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

func CategoryUnifier(category, target string) string {
	// First, check the target format for wildcards, as this is the most reliable indicator.
	if strings.HasPrefix(target, "*.") {
		return "wildcard"
	}

	// Normalize category string for matching
	catLower := strings.ToLower(category)

	switch catLower {
	case "wildcard":
		return "wildcard"
	case "url", "website", "web", "web-application", "api", "ip_address", "ip-address":
		return "url"
	case "cidr", "iprange":
		return "cidr"
	case "android", "google_play_app_id", "other_apk", "mobile-application-android", "mobile-application":
		return "android"
	case "ios", "apple_store_app_id", "other_ipa", "testflight", "mobile-application-ios", "apple-store":
		return "ios"
	case "ai_model":
		return "ai"
	case "hardware", "device", "iot":
		return "hardware"
	case "smart_contract":
		return "blockchain"
	case "windows_app_store_app_id", "downloadable_executables":
		return "binary"
	case "source_code":
		return "code"
	case "other", "aws_cloud_config", "application", "network":
		return "other"
	}

	// For anything else, just format it nicely.
	return strings.ReplaceAll(catLower, "_", " ")
}
