package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

var getWildcardsCmd = &cobra.Command{
	Use:   "wildcards",
	Short: "Get all targets that are wildcards",
	RunE:  runGetWildcardsCmd,
}

func init() {
	getWildcardsCmd.Flags().BoolP("aggressive", "a", false, "Extract root domains from all URL targets in addition to wildcards.")
	getWildcardsCmd.Flags().String("platform", "all", "Limit results to a specific platform (e.g. h1, bugcrowd, intigriti).")
	getWildcardsCmd.Flags().StringP("output", "o", "t", "Output flags. Supported: t (target), u (program URL). Example: -o tu")
	getWildcardsCmd.Flags().StringP("delimiter", "d", " ", "Delimiter to use between output fields.")
	getCmd.AddCommand(getWildcardsCmd)
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

	db, err := storage.Open(dbPath, storage.DefaultDBTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	platformFilter, _ := cmd.Flags().GetString("platform")
	listOpts := storage.ListOptions{
		Platform:   platformFilter,
		IncludeOOS: true,
	}

	entries, err := db.ListEntries(context.Background(), listOpts)
	if err != nil {
		return err
	}

	aggressive, _ := cmd.Flags().GetBool("aggressive")
	outputFlags, _ := cmd.Flags().GetString("output")
	if outputFlags == "" {
		outputFlags = "t"
	}
	for _, flag := range outputFlags {
		if flag != 't' && flag != 'u' {
			return fmt.Errorf("invalid output flag '%c'. Supported flags: t, u", flag)
		}
	}
	delimiter, _ := cmd.Flags().GetString("delimiter")

	domainPrograms := collectWildcards(entries, aggressive)
	domains := make([]string, 0, len(domainPrograms))
	for domain := range domainPrograms {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	includeProgram := strings.ContainsRune(outputFlags, 'u')

	for _, domain := range domains {
		programs := domainPrograms[domain]
		if includeProgram {
			programList := make([]string, 0, len(programs))
			for programURL := range programs {
				programList = append(programList, programURL)
			}
			sort.Strings(programList)
			for _, programURL := range programList {
				line := formatWildcardLine(domain, programURL, outputFlags, delimiter)
				if line != "" {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
			}
			continue
		}

		line := formatWildcardLine(domain, "", outputFlags, delimiter)
		if line != "" {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}

	return nil
}

var blacklistedSuffixes = []string{
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
	"adobeaemcloud.com",
	"azurefd.net",
	"windows.net",
	"strapiapp.com",
	"forgeblocks.com",
}

var nonDomainCategories = map[string]struct{}{
	"android":    {},
	"ios":        {},
	"binary":     {},
	"code":       {},
	"ai":         {},
	"hardware":   {},
	"blockchain": {},
}

func collectWildcards(entries []storage.Entry, aggressive bool) map[string]map[string]struct{} {
	uniqueDomains := make(map[string]map[string]struct{})

	outOfScopeByProgram := make(map[string]map[string]struct{})
	for _, e := range entries {
		if e.InScope {
			continue
		}
		if !strings.Contains(e.TargetNormalized, "*") {
			continue
		}
		if wildcardHasPath(e.TargetNormalized) {
			continue
		}

		normalizedOOS := normalizeForSubdomainTools(e.TargetNormalized)
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

		cleanHost := normalizeForSubdomainTools(e.TargetNormalized)
		if cleanHost == "" {
			continue
		}

		if isBlacklistedSuffix(cleanHost) {
			continue
		}

		var finalDomain string
		isExplicitWildcard := e.Category == "wildcard" || strings.Contains(e.TargetNormalized, "*")

		if isExplicitWildcard {
			normalized := normalizeForSubdomainTools(e.TargetNormalized)
			if root, ok := storage.ExtractRootDomain(normalized); ok {
				finalDomain = root
			} else {
				utils.Log.Debug("[skip] ", e.TargetNormalized)
			}
		} else if aggressive {
			category := strings.ToLower(e.Category)
			target := strings.ToLower(e.TargetNormalized)

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

			normalized := normalizeForSubdomainTools(target)
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

func formatWildcardLine(domain, programURL, outputFlags, delimiter string) string {
	var builder strings.Builder
	for _, flag := range outputFlags {
		switch flag {
		case 't':
			builder.WriteString(domain)
			builder.WriteString(delimiter)
		case 'u':
			builder.WriteString(programURL)
			builder.WriteString(delimiter)
		}
	}
	line := builder.String()
	line = strings.TrimSuffix(line, delimiter)
	return line
}

func wildcardHasPath(target string) bool {
	schemeStripped := target
	if i := strings.Index(schemeStripped, "://"); i != -1 {
		schemeStripped = schemeStripped[i+3:]
	}
	return strings.Contains(schemeStripped, "/")
}

func isBlacklistedSuffix(host string) bool {
	for _, suffix := range blacklistedSuffixes {
		if strings.HasSuffix(host, "."+suffix) || host == suffix {
			return true
		}
	}
	return false
}

func normalizeForSubdomainTools(scope string) string {
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
