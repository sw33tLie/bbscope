package targets

import (
	"net"
	"net/url"
	"strings"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

// CollectIPs returns in-scope targets that are IP addresses.
// For URL targets, the host is extracted if it's an IP.
func CollectIPs(entries []storage.Entry) []string {
	return collectByType(entries, true, nil, extractIP)
}

// CollectOOSIPs returns out-of-scope targets that are IP addresses.
func CollectOOSIPs(entries []storage.Entry) []string {
	return collectByType(entries, false, nil, extractIP)
}

func extractIP(target string) string {
	if isIP(target) {
		return target
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if u, err := url.Parse(target); err == nil {
			host := strings.Trim(u.Hostname(), "[]")
			if isIP(host) {
				return host
			}
		}
	}
	return ""
}

func isIP(s string) bool {
	s = strings.Trim(s, "[]")
	return net.ParseIP(s) != nil
}
