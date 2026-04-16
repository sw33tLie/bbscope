package targets

import (
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

func TestCollectOOSWildcards_PartialWildcards(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.something.test.com",
			Category:         "wildcard",
			InScope:          false,
		},
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.test.com",
			Category:         "wildcard",
			InScope:          true, // in-scope, should be ignored
		},
	}

	got := CollectOOSWildcards(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 OOS wildcard, got %d: %#v", len(got), got)
	}
	if _, ok := got["something.test.com"]; !ok {
		t.Fatalf("expected 'something.test.com', got %#v", got)
	}
}

func TestCollectOOSWildcards_MultiplePrograms(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/prog1",
			TargetNormalized: "*.internal.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
		{
			ProgramURL:       "https://hackerone.com/prog2",
			TargetNormalized: "*.internal.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	got := CollectOOSWildcards(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(got))
	}
	programs := got["internal.example.com"]
	if len(programs) != 2 {
		t.Fatalf("expected 2 programs, got %d", len(programs))
	}
}

func TestCollectOOSWildcards_SkipsInScope(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com",
			Category:         "wildcard",
			InScope:          true,
		},
	}

	got := CollectOOSWildcards(entries)
	if len(got) != 0 {
		t.Fatalf("expected 0 OOS wildcards for in-scope entry, got %d", len(got))
	}
}

func TestCollectOOSWildcards_SkipsNonWildcards(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "example.com",
			Category:         "domain",
			InScope:          false,
		},
	}

	got := CollectOOSWildcards(entries)
	if len(got) != 0 {
		t.Fatalf("expected 0 OOS wildcards for non-wildcard entry, got %d", len(got))
	}
}

func TestCollectOOSWildcards_SkipsWildcardsWithPath(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com/api",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	got := CollectOOSWildcards(entries)
	if len(got) != 0 {
		t.Fatalf("expected 0 OOS wildcards for wildcard with path, got %d", len(got))
	}
}

func TestCollectOOSWildcards_RootWildcard(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	got := CollectOOSWildcards(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 OOS wildcard, got %d", len(got))
	}
	if _, ok := got["example.com"]; !ok {
		t.Fatalf("expected 'example.com', got %#v", got)
	}
}

func TestCollectOOSWildcardsSorted(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/prog1",
			TargetNormalized: "*.z.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
		{
			ProgramURL:       "https://hackerone.com/prog2",
			TargetNormalized: "*.a.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
		{
			ProgramURL:       "https://hackerone.com/prog1",
			TargetNormalized: "*.m.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	results := CollectOOSWildcardsSorted(entries)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Domain != "a.example.com" {
		t.Fatalf("expected first domain 'a.example.com', got '%s'", results[0].Domain)
	}
	if results[1].Domain != "m.example.com" {
		t.Fatalf("expected second domain 'm.example.com', got '%s'", results[1].Domain)
	}
	if results[2].Domain != "z.example.com" {
		t.Fatalf("expected third domain 'z.example.com', got '%s'", results[2].Domain)
	}
}

// A strict program with only url-category targets must not contribute any
// domains in aggressive mode.
func TestCollectWildcards_StrictProgramSkipsAggressiveURL(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "example.org",
			Category:         "url",
			InScope:          true,
			Strict:           true,
		},
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "https://api.example.net",
			Category:         "url",
			InScope:          true,
			Strict:           true,
		},
	}

	got := CollectWildcards(entries, WildcardOptions{Aggressive: true})
	if len(got) != 0 {
		t.Fatalf("expected 0 domains for strict program with only urls, got %d: %#v", len(got), got)
	}
}

// A strict program with an explicit wildcard must still contribute that
// wildcard's root domain. Strict only suppresses aggressive url extraction,
// not explicit wildcard entries.
func TestCollectWildcards_StrictProgramKeepsExplicitWildcard(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "*.example.net",
			Category:         "wildcard",
			InScope:          true,
			Strict:           true,
		},
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "example.org",
			Category:         "url",
			InScope:          true,
			Strict:           true,
		},
	}

	got := CollectWildcards(entries, WildcardOptions{Aggressive: true})
	if len(got) != 1 {
		t.Fatalf("expected 1 domain (from explicit wildcard), got %d: %#v", len(got), got)
	}
	if _, ok := got["example.net"]; !ok {
		t.Fatalf("expected example.net from explicit wildcard, got %#v", got)
	}
	if _, ok := got["example.org"]; ok {
		t.Fatalf("example.org must not be extracted for strict program, got %#v", got)
	}
}

// A domain extracted aggressively from a non-strict program must still be
// emitted even if a strict program also references it. The strict program's
// ProgramURL should not be attached to that domain (because its url entry
// was skipped), but the domain itself must survive.
func TestCollectWildcards_StrictDoesNotHideDomainFromOtherPrograms(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "example.org",
			Category:         "url",
			InScope:          true,
			Strict:           true,
		},
		{
			ProgramURL:       "https://example.com/prog2",
			TargetNormalized: "*.example.org",
			Category:         "wildcard",
			InScope:          true,
			Strict:           false,
		},
	}

	got := CollectWildcards(entries, WildcardOptions{Aggressive: true})
	programs, ok := got["example.org"]
	if !ok {
		t.Fatalf("example.org must still be emitted from non-strict program, got %#v", got)
	}
	if _, ok := programs["https://example.com/prog2"]; !ok {
		t.Fatalf("expected non-strict program attached to example.org, got %#v", programs)
	}
	if _, ok := programs["https://example.com/prog1"]; ok {
		t.Fatalf("strict program must not be attached to example.org (its url entry was skipped), got %#v", programs)
	}
}

// Regression: non-strict programs must keep the old aggressive-extraction
// behaviour — a url-category target still produces its root domain.
func TestCollectWildcards_NonStrictAggressiveUnchanged(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "example.org",
			Category:         "url",
			InScope:          true,
			Strict:           false,
		},
	}

	got := CollectWildcards(entries, WildcardOptions{Aggressive: true})
	if _, ok := got["example.org"]; !ok {
		t.Fatalf("non-strict aggressive extraction must still emit example.org, got %#v", got)
	}
}

// Strict flag must have no effect when aggressive mode is disabled. Explicit
// wildcards are still collected regardless of strict.
func TestCollectWildcards_StrictIgnoredWhenNonAggressive(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "*.example.net",
			Category:         "wildcard",
			InScope:          true,
			Strict:           true,
		},
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "example.org",
			Category:         "url",
			InScope:          true,
			Strict:           true,
		},
	}

	got := CollectWildcards(entries, WildcardOptions{Aggressive: false})
	if len(got) != 1 {
		t.Fatalf("expected 1 domain (the explicit wildcard) in non-aggressive mode, got %d: %#v", len(got), got)
	}
	if _, ok := got["example.net"]; !ok {
		t.Fatalf("expected example.net from explicit wildcard, got %#v", got)
	}
}

// The OOS-wildcard block must still apply to a strict program's in-scope
// wildcards. Verifies the strict flag doesn't accidentally bypass the OOS
// check.
func TestCollectWildcards_StrictDoesNotBypassOOSCheck(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "*.example.net",
			Category:         "wildcard",
			InScope:          true,
			Strict:           true,
		},
		{
			ProgramURL:       "https://example.com/prog1",
			TargetNormalized: "*.example.net",
			Category:         "wildcard",
			InScope:          false,
			Strict:           true,
		},
	}

	got := CollectWildcards(entries, WildcardOptions{Aggressive: true})
	if _, ok := got["example.net"]; ok {
		t.Fatalf("OOS wildcard must still block the domain even for strict program, got %#v", got)
	}
}
