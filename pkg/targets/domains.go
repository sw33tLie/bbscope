package targets

import (
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// CollectDomains returns in-scope targets that are domains (not URLs, not IPs, not CIDRs).
func CollectDomains(entries []storage.Entry) []string {
	return collectByType(entries, true, isDomain)
}

// CollectOOSDomains returns out-of-scope targets that are domains.
func CollectOOSDomains(entries []storage.Entry) []string {
	return collectByType(entries, false, isDomain)
}

func isDomain(target string) bool {
	return strings.Contains(target, ".") &&
		!strings.HasPrefix(target, "http://") &&
		!strings.HasPrefix(target, "https://")
}
