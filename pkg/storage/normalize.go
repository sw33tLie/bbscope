package storage

import (
	"net/url"
	"strings"
)

// NormalizeTarget applies simple canonicalization rules suitable for identity.
func NormalizeTarget(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// If it looks like a URL, normalize scheme/host/trailing slash.
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		u.Host = strings.ToLower(u.Host)
		if u.Scheme == "http" && u.Port() == "80" {
			u.Host = strings.TrimSuffix(u.Host, ":80")
		}
		if u.Scheme == "https" && u.Port() == "443" {
			u.Host = strings.TrimSuffix(u.Host, ":443")
		}
		if strings.HasSuffix(u.Path, "/") && len(u.Path) > 1 {
			u.Path = strings.TrimRight(u.Path, "/")
		}
		if u.Scheme == "" {
			u.Scheme = "https"
		}
		return u.String()
	}
	// Wildcards/domains
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, ".")
	if strings.HasSuffix(s, "/") {
		s = strings.TrimRight(s, "/")
	}
	return s
}

// NormalizeProgramURL ensures consistent program URL identity.
func NormalizeProgramURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		u.Host = strings.ToLower(u.Host)
		if strings.HasSuffix(u.Path, "/") && len(u.Path) > 1 {
			u.Path = strings.TrimRight(u.Path, "/")
		}
		if u.Scheme == "" {
			u.Scheme = "https"
		}
		return u.String()
	}
	return s
}
