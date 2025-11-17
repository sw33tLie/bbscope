package ai

import (
	"reflect"
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

func TestMergeNormalizedPreservesMetadataAndExpands(t *testing.T) {
	input := []storage.TargetItem{
		{URI: "example.*", Category: "wildcard", Description: "main", InScope: true},
		{URI: "example.(it|com)", Category: "url", Description: "alt", InScope: false},
	}

	normalized := map[int]normalizedResult{
		5: {Targets: []string{"example.com"}},
		6: {Targets: []string{"example.it", " example.com "}},
	}

	out := mergeNormalized(input, 5, normalized)

	if len(out) != 3 {
		t.Fatalf("expected 3 items, got %d", len(out))
	}

	expect := []storage.TargetItem{
		{URI: "example.com", Category: "wildcard", Description: "main", InScope: true},
		{URI: "example.it", Category: "url", Description: "alt", InScope: false},
		{URI: "example.com", Category: "url", Description: "alt", InScope: false},
	}

	if !reflect.DeepEqual(out, expect) {
		t.Fatalf("unexpected merge result:\nwant %#v\n got %#v", expect, out)
	}
}

func TestMergeNormalizedFallsBackToOriginal(t *testing.T) {
	input := []storage.TargetItem{
		{URI: "original", Category: "url"},
	}
	normalized := map[int]normalizedResult{}

	out := mergeNormalized(input, 0, normalized)
	if len(out) != 1 {
		t.Fatalf("expected single item fallback, got %d", len(out))
	}
	if out[0].URI != "original" {
		t.Fatalf("expected fallback URI 'original', got %s", out[0].URI)
	}
}

func TestSanitizeTargetsDeduplicatesAndLowercases(t *testing.T) {
	in := []string{"Example.COM ", " example.com", "  "}
	out := sanitizeTargets(in)

	if len(out) != 1 {
		t.Fatalf("expected one sanitized target, got %d", len(out))
	}
	if out[0] != "example.com" {
		t.Fatalf("expected example.com, got %s", out[0])
	}
}

func TestMergeNormalizedOverridesInScope(t *testing.T) {
	falseVal := false
	input := []storage.TargetItem{
		{URI: "example.com", InScope: true},
	}
	normalized := map[int]normalizedResult{
		0: {Targets: []string{"example.com"}, InScope: &falseVal},
	}

	out := mergeNormalized(input, 0, normalized)
	if len(out) != 1 {
		t.Fatalf("expected one item, got %d", len(out))
	}
	if out[0].InScope {
		t.Fatalf("expected InScope overridden to false")
	}
}
