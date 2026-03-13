// Package targets provides functions for extracting specific target types
// (URLs, IPs, CIDRs, domains, wildcards) from scope entries.
package targets

import (
	"sort"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// matchFunc returns true if the target string matches the desired type.
type matchFunc func(target string) bool

// extractFunc returns the extracted value from a target, or "" if not applicable.
type extractFunc func(target string) string

// collectByType filters entries by scope and target type, returning unique sorted results.
// If extract is provided, it is used to derive the value from the target (e.g. extracting
// an IP from a URL). Otherwise, match is used to test the target directly.
func collectByType(entries []storage.Entry, inScope bool, match matchFunc, extract ...extractFunc) []string {
	seen := make(map[string]struct{})

	for _, e := range entries {
		if e.InScope != inScope {
			continue
		}

		target := e.TargetNormalized

		if len(extract) > 0 && extract[0] != nil {
			if v := extract[0](target); v != "" {
				if _, exists := seen[v]; !exists {
					seen[v] = struct{}{}
				}
			}
			continue
		}

		if match != nil && match(target) {
			if _, exists := seen[target]; !exists {
				seen[target] = struct{}{}
			}
		}
	}

	results := make([]string, 0, len(seen))
	for t := range seen {
		results = append(results, t)
	}
	sort.Strings(results)
	return results
}
