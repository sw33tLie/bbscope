package wildcards

import (
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

func TestCollectOOS_PartialWildcards(t *testing.T) {
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

	got := CollectOOS(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 OOS wildcard, got %d: %#v", len(got), got)
	}
	if _, ok := got["something.test.com"]; !ok {
		t.Fatalf("expected 'something.test.com', got %#v", got)
	}
}

func TestCollectOOS_MultiplePrograms(t *testing.T) {
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

	got := CollectOOS(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(got))
	}
	programs := got["internal.example.com"]
	if len(programs) != 2 {
		t.Fatalf("expected 2 programs, got %d", len(programs))
	}
}

func TestCollectOOS_SkipsInScope(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com",
			Category:         "wildcard",
			InScope:          true,
		},
	}

	got := CollectOOS(entries)
	if len(got) != 0 {
		t.Fatalf("expected 0 OOS wildcards for in-scope entry, got %d", len(got))
	}
}

func TestCollectOOS_SkipsNonWildcards(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "example.com",
			Category:         "domain",
			InScope:          false,
		},
	}

	got := CollectOOS(entries)
	if len(got) != 0 {
		t.Fatalf("expected 0 OOS wildcards for non-wildcard entry, got %d", len(got))
	}
}

func TestCollectOOS_SkipsWildcardsWithPath(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com/api",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	got := CollectOOS(entries)
	if len(got) != 0 {
		t.Fatalf("expected 0 OOS wildcards for wildcard with path, got %d", len(got))
	}
}

func TestCollectOOS_RootWildcard(t *testing.T) {
	entries := []storage.Entry{
		{
			ProgramURL:       "https://hackerone.com/example",
			TargetNormalized: "*.example.com",
			Category:         "wildcard",
			InScope:          false,
		},
	}

	got := CollectOOS(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 OOS wildcard, got %d", len(got))
	}
	if _, ok := got["example.com"]; !ok {
		t.Fatalf("expected 'example.com', got %#v", got)
	}
}

func TestCollectOOSSorted(t *testing.T) {
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

	results := CollectOOSSorted(entries)
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
