package ai

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sw33tLie/bbscope/v2/pkg/storage"
)

func TestNormalizerScenarios(t *testing.T) {
	falseVal := false
	tests := []struct {
		name     string
		baseID   int
		input    []storage.TargetItem
		norm     map[int]normalizedResult
		expected []storage.TargetItem
	}{
		{
			name:   "expands targets and keeps metadata",
			baseID: 5,
			input: []storage.TargetItem{
				{URI: "example.*", Category: "wildcard", Description: "main", InScope: true},
				{URI: "example.(it|com)", Category: "url", Description: "alt", InScope: false},
			},
			norm: map[int]normalizedResult{
				5: {Targets: []string{"example.com"}},
				6: {Targets: []string{"example.it", " example.com "}},
			},
			expected: []storage.TargetItem{
				{
					URI:         "example.*",
					Category:    "wildcard",
					Description: "main",
					InScope:     true,
					Variants: []storage.TargetVariant{
						{Value: "example.com"},
					},
				},
				{
					URI:         "example.(it|com)",
					Category:    "url",
					Description: "alt",
					InScope:     false,
					Variants: []storage.TargetVariant{
						{Value: "example.it"},
						{Value: "example.com"},
					},
				},
			},
		},
		{
			name:   "falls back to original when missing",
			baseID: 0,
			input: []storage.TargetItem{
				{URI: "original", Category: "url"},
			},
			norm: map[int]normalizedResult{},
			expected: []storage.TargetItem{
				{URI: "original", Category: "url"},
			},
		},
		{
			name:   "overrides in scope",
			baseID: 0,
			input: []storage.TargetItem{
				{URI: "example.com", Category: "url", InScope: true},
			},
			norm: map[int]normalizedResult{
				0: {Targets: []string{"example.com"}, InScope: &falseVal},
			},
			expected: []storage.TargetItem{
				{
					URI:      "example.com",
					Category: "url",
					InScope:  false,
					Variants: []storage.TargetVariant{
						{Value: "example.com", HasInScope: true, InScope: false},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out := mergeNormalized(tc.input, tc.baseID, tc.norm)
			if !reflect.DeepEqual(out, tc.expected) {
				t.Fatalf("input:\n%s\nexpected:\n%s\nactual:\n%s",
					mustJSON(tc.input), mustJSON(tc.expected), mustJSON(out))
			}
		})
	}

	t.Run("sanitize deduplicates", func(t *testing.T) {
		in := []string{"Example.COM ", " example.com", "  "}
		out := sanitizeTargets(in)
		if len(out) != 1 || out[0] != "example.com" {
			t.Fatalf("sanitize failed: %v", out)
		}
	})
}

func mustJSON(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
