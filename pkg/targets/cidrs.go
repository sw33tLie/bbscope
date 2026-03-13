package targets

import (
	"net"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// CollectCIDRs returns in-scope targets that are CIDR ranges or IP ranges.
func CollectCIDRs(entries []storage.Entry) []string {
	return collectByType(entries, true, isCIDROrRange)
}

// CollectOOSCIDRs returns out-of-scope targets that are CIDR ranges or IP ranges.
func CollectOOSCIDRs(entries []storage.Entry) []string {
	return collectByType(entries, false, isCIDROrRange)
}

func isCIDROrRange(target string) bool {
	return isCIDR(target) || isIPRange(target)
}

func isCIDR(s string) bool {
	if !strings.Contains(s, "/") {
		return false
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

func isIPRange(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return false
	}
	return net.ParseIP(strings.TrimSpace(parts[0])) != nil &&
		net.ParseIP(strings.TrimSpace(parts[1])) != nil
}
