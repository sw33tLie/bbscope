package storage

import (
	"net/url"
	"strings"

	"github.com/weppos/publicsuffix-go/publicsuffix"
)

// AggressiveTransform takes a scope string and applies aggressive transformations
// to extract a root domain and return it as a wildcard.
// e.g., "sub.domain.com" -> "*.domain.com"
func AggressiveTransform(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return ""
	}

	// If it's a URL, get the host.
	host := scope
	if u, err := url.Parse(scope); err == nil && u.Host != "" {
		host = u.Hostname()
	}

	// This is not a domain
	if !strings.Contains(host, ".") {
		return scope
	}

	// Don't transform wildcards
	if strings.Contains(host, "*") {
		return scope
	}

	domain, err := publicsuffix.Domain(host)
	if err != nil {
		// Couldn't parse, return original scope
		return scope
	}

	return "*." + domain
}

// ExtractRootDomain takes a scope string and tries to find the root domain.
// e.g., "http://sub.foo.example.co.uk/path" -> "example.co.uk", true
func ExtractRootDomain(scope string) (string, bool) {
	host := scope

	// If a scope string looks like a domain but lacks a scheme, url.Parse can fail to
	// identify the host. Prepending a scheme makes parsing more reliable.
	if !strings.Contains(scope, "://") && strings.Contains(scope, ".") {
		scope = "http://" + scope
	}

	if u, err := url.Parse(scope); err == nil && u.Host != "" {
		host = u.Hostname()
	} else {
		// Fallback for things that are not valid URLs
		host = strings.Split(host, "/")[0]
		host = strings.Split(host, ":")[0]
		if !strings.Contains(host, ".") {
			return "", false
		}
	}

	// Don't extract a root domain from something that is already a wildcard.
	if strings.Contains(host, "*") {
		return "", false
	}

	domain, err := publicsuffix.Domain(host)
	if err != nil {
		return "", false
	}

	return domain, true
}
