// Package wildcards provides functionality for extracting and processing
// wildcard domains from bug bounty scope entries.
package wildcards

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// Options configures how wildcards are collected.
type Options struct {
	// Aggressive extracts root domains from all URL targets, not just wildcards.
	Aggressive bool
}

// Result represents a wildcard domain with its associated programs.
type Result struct {
	Domain      string
	ProgramURLs []string
}

// BlacklistedSuffixes contains domain suffixes that are typically not useful
// for subdomain enumeration (shared hosting, cloud providers, etc.).
var BlacklistedSuffixes = []string{
	"amazonaws.com",
	"amazoncognito.com",
	"azurewebsites.net",
	"azure.com",
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
	"firebase.app",
	"run.app",
	"adobeaemcloud.com",
	"firebaseapp.com",
	"web.app",
	"azurefd.net",
	"windows.net",
	"strapiapp.com",
	"forgeblocks.com",
}

// NonDomainCategories contains scope categories that don't represent domains.
var NonDomainCategories = map[string]struct{}{
	"android":    {},
	"ios":        {},
	"binary":     {},
	"code":       {},
	"ai":         {},
	"hardware":   {},
	"blockchain": {},
}

// Collect extracts wildcard domains from the given entries.
// Returns a map of domain -> set of program URLs.
func Collect(entries []storage.Entry, opts Options) map[string]map[string]struct{} {
	uniqueDomains := make(map[string]map[string]struct{})

	outOfScopeByProgram := make(map[string]map[string]struct{})
	for _, e := range entries {
		if e.InScope {
			continue
		}
		if !strings.Contains(e.TargetNormalized, "*") {
			continue
		}
		if WildcardHasPath(e.TargetNormalized) {
			continue
		}

		normalizedOOS := NormalizeForSubdomainTools(e.TargetNormalized)
		if normalizedOOS == "" {
			continue
		}
		if _, ok := outOfScopeByProgram[e.ProgramURL]; !ok {
			outOfScopeByProgram[e.ProgramURL] = make(map[string]struct{})
		}
		outOfScopeByProgram[e.ProgramURL][normalizedOOS] = struct{}{}
	}

	for _, e := range entries {
		if !e.InScope {
			utils.Log.Debug("[skip-oos] ", e.TargetNormalized)
			continue
		}

		if strings.Contains(e.TargetNormalized, " ") {
			utils.Log.Debug("[skip-space] ", e.TargetNormalized)
			continue
		}

		cleanHost := NormalizeForSubdomainTools(e.TargetNormalized)
		if cleanHost == "" {
			continue
		}

		if IsBlacklistedSuffix(cleanHost) {
			continue
		}

		var finalDomain string
		isExplicitWildcard := e.Category == "wildcard" || strings.Contains(e.TargetNormalized, "*")

		if isExplicitWildcard {
			normalized := NormalizeForSubdomainTools(e.TargetNormalized)
			if root, ok := storage.ExtractRootDomain(normalized); ok {
				finalDomain = root
			} else {
				utils.Log.Debug("[skip] ", e.TargetNormalized)
			}
		} else if opts.Aggressive {
			category := strings.ToLower(e.Category)
			target := strings.ToLower(e.TargetNormalized)

			if _, found := NonDomainCategories[category]; found {
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

			normalized := NormalizeForSubdomainTools(target)
			if rootDomain, ok := storage.ExtractRootDomain(normalized); ok {
				finalDomain = rootDomain
			}
		}

		if finalDomain == "" {
			continue
		}

		if programOOS, programExists := outOfScopeByProgram[e.ProgramURL]; programExists {
			if _, isOOS := programOOS[finalDomain]; isOOS {
				continue
			}
		}
		if _, exists := uniqueDomains[finalDomain]; !exists {
			uniqueDomains[finalDomain] = make(map[string]struct{})
		}
		uniqueDomains[finalDomain][e.ProgramURL] = struct{}{}
	}

	return uniqueDomains
}

// CollectSorted is a convenience function that returns sorted Results
// instead of a raw map.
func CollectSorted(entries []storage.Entry, opts Options) []Result {
	domainPrograms := Collect(entries, opts)

	domains := make([]string, 0, len(domainPrograms))
	for domain := range domainPrograms {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	results := make([]Result, 0, len(domains))
	for _, domain := range domains {
		programs := domainPrograms[domain]
		programList := make([]string, 0, len(programs))
		for programURL := range programs {
			programList = append(programList, programURL)
		}
		sort.Strings(programList)

		results = append(results, Result{
			Domain:      domain,
			ProgramURLs: programList,
		})
	}

	return results
}

// WildcardHasPath returns true if the target contains a path after the host.
func WildcardHasPath(target string) bool {
	schemeStripped := target
	if i := strings.Index(schemeStripped, "://"); i != -1 {
		schemeStripped = schemeStripped[i+3:]
	}
	return strings.Contains(schemeStripped, "/")
}

// IsBlacklistedSuffix returns true if the host ends with a blacklisted suffix.
func IsBlacklistedSuffix(host string) bool {
	for _, suffix := range BlacklistedSuffixes {
		if strings.HasSuffix(host, "."+suffix) || host == suffix {
			return true
		}
	}
	return false
}

// NormalizeForSubdomainTools cleans up a scope string for use with
// subdomain enumeration tools.
func NormalizeForSubdomainTools(scope string) string {
	var processingStr string
	if u, err := url.Parse(scope); err == nil && u.Host != "" {
		processingStr = u.Host
	} else {
		processingStr = scope
	}

	processingStr = strings.Split(processingStr, "/")[0]
	processingStr = strings.Split(processingStr, ":")[0]

	if strings.HasSuffix(processingStr, ".*") {
		processingStr = strings.TrimSuffix(processingStr, ".*") + ".com"
	}

	if strings.HasSuffix(processingStr, ".<tld>") {
		processingStr = strings.TrimSuffix(processingStr, ".<tld>") + ".com"
	}

	processingStr = strings.ReplaceAll(processingStr, "*", "")
	processingStr = strings.ReplaceAll(processingStr, ",", ".")
	processingStr = strings.TrimPrefix(processingStr, ".")
	processingStr = strings.ReplaceAll(processingStr, "(", "")
	processingStr = strings.ReplaceAll(processingStr, ")", "")
	processingStr = regexp.MustCompile(`\[.*?\]`).ReplaceAllString(processingStr, "")
	processingStr = strings.Trim(processingStr, ". ")

	return processingStr
}
