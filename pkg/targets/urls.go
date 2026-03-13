package targets

import (
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// CollectURLs returns in-scope targets that are URLs (http:// or https://).
func CollectURLs(entries []storage.Entry) []string {
	return collectByType(entries, true, isURL)
}

// CollectOOSURLs returns out-of-scope targets that are URLs.
func CollectOOSURLs(entries []storage.Entry) []string {
	return collectByType(entries, false, isURL)
}

func isURL(target string) bool {
	return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
}
