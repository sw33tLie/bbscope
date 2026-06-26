package bugcrowd

import "testing"

func TestNormalizeBugcrowdHandle(t *testing.T) {
	tests := map[string]string{
		"/engagements/example":                             "/engagements/example",
		"https://bugcrowd.com/engagements/example":         "/engagements/example",
		"https://bugcrowd.com/engagements/example?foo=bar": "/engagements/example?foo=bar",
		"bugcrowd.com/engagements/example":                 "/engagements/example",
	}

	for input, want := range tests {
		if got := normalizeBugcrowdHandle(input); got != want {
			t.Fatalf("normalizeBugcrowdHandle(%q) = %q, want %q", input, got, want)
		}
	}
}
