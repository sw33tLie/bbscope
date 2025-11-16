package cmd

import (
	"reflect"
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
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

	got := collectWildcards(entries, false)
	expect := []string{"example.com"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("unexpected domains.\nwant: %#v\ngot:  %#v", expect, got)
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

	got := collectWildcards(entries, true)
	expect := []string{"test.ai"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("unexpected domains.\nwant: %#v\ngot:  %#v", expect, got)
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

	got := collectWildcards(entries, true)
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

	if got := collectWildcards(entries, false); len(got) != 0 {
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

	got := collectWildcards(entries, true)
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

	got := collectWildcards(entries, true)
	expect := []string{"example.com"}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("expected deduped sorted domains %#v, got %#v", expect, got)
	}
}
