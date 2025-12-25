package cmd

import (
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
	"github.com/sw33tLie/bbscope/v2/pkg/wildcards"
)

func TestCollectWildcards_NonAggressiveExplicitOnly(t *testing.T) {
	t.Helper()

	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com",
			Category:         "wildcard",
			InScope:          true,
		},
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "https://portal.example.com/login",
			Category:         "url",
			InScope:          true,
		},
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "https://11.222.33.44/accounts/login?local=true",
			Category:         "url",
			InScope:          true,
		},
	}

	got := wildcards.Collect(entries, wildcards.Options{Aggressive: false})
	if len(got) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(got))
	}
	programs, ok := got["example.com"]
	if !ok {
		t.Fatalf("expected example.com domain")
	}
	if _, ok := programs["https://hackerone.com/example"]; !ok {
		t.Fatalf("expected program url for example.com")
	}
}

func TestCollectWildcardsAggressiveSkipsIPs(t *testing.T) {
	t.Helper()

	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/test_bbp",
			TargetNormalized: "https://portal.test.ai/login",
			Category:         "url",
			InScope:          true,
		},
		{
			ProgramURL:       "https://hackerone.com/test_bbp",
			TargetNormalized: "https://12.34.43.12/accounts/login?local=true",
			Category:         "url",
			InScope:          true,
		},
	}

	got := wildcards.Collect(entries, wildcards.Options{Aggressive: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(got))
	}
	programs, ok := got["test.ai"]
	if !ok {
		t.Fatalf("expected test.ai domain")
	}
	if _, ok := programs["https://hackerone.com/test_bbp"]; !ok {
		t.Fatalf("expected program url for test.ai")
	}
}

func TestCollectWildcardsAggressiveRespectsOutOfScope(t *testing.T) {
	t.Helper()

	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/blocked",
			TargetNormalized: "https://portal.blocked.example.com/login",
			Category:         "url",
			InScope:          true,
		},
		{
			ProgramURL:       "https://hackerone.com/blocked",
			TargetNormalized: "*.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	got := wildcards.Collect(entries, wildcards.Options{Aggressive: true})
	if len(got) != 0 {
		t.Fatalf("expected no domains when OOS wildcard blocks them, got %#v", got)
	}
}

func TestCollectWildcardsSkipsBlacklistedSuffixes(t *testing.T) {
	t.Helper()

	entries := []storage.Entry{
		{
			ProgramURL:       "https://bugcrowd.com/app",
			TargetNormalized: "*.cloudfront.net",
			Category:         "wildcard",
			InScope:          true,
		},
	}

	if got := wildcards.Collect(entries, wildcards.Options{Aggressive: false}); len(got) != 0 {
		t.Fatalf("expected cloudfront.net to be filtered but got %#v", got)
	}
}

func TestCollectWildcardsAggressiveSkipsNonDomainCategories(t *testing.T) {
	t.Helper()

	entries := []storage.Entry{
		{
			ProgramURL:       "https://intigriti.com/mobile",
			TargetNormalized: "mobile-app.example.com",
			Category:         "android",
			InScope:          true,
		},
	}

	got := wildcards.Collect(entries, wildcards.Options{Aggressive: true})
	if len(got) != 0 {
		t.Fatalf("android entries should be ignored in aggressive mode, got %#v", got)
	}
}

func TestCollectWildcardsDeduplicatesAndSorts(t *testing.T) {
	t.Helper()

	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/sdgsdfa",
			TargetNormalized: "*.beta.example.com",
			Category:         "wildcard",
			InScope:          true,
		},
		{
			ProgramURL:       "https://hackerone.com/sdgsdfa",
			TargetNormalized: "https://portal.example.com/login",
			Category:         "url",
			InScope:          true,
		},
		{
			ProgramURL:       "https://hackerone.com/sdgsdfa",
			TargetNormalized: "https://shop.alpha.example.com",
			Category:         "url",
			InScope:          true,
		},
	}

	got := wildcards.Collect(entries, wildcards.Options{Aggressive: true})
	programs, ok := got["example.com"]
	if !ok || len(got) != 1 {
		t.Fatalf("expected only example.com domain, got %#v", got)
	}
	if _, ok := programs["https://hackerone.com/sdgsdfa"]; !ok {
		t.Fatalf("expected program for example.com dedupe test")
	}
}
