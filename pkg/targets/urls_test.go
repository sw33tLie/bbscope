package targets

import "testing"

func TestCollectURLs(t *testing.T) {
	ents := mkEntries(
		e("https://example.com/app", true),
		e("http://test.com", true),
		e("example.com", true),
		e("https://oos.com", false),
	)

	got := CollectURLs(ents)
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(got), got)
	}
}

func TestCollectOOSURLs(t *testing.T) {
	ents := mkEntries(
		e("https://example.com/app", true),
		e("https://oos.com", false),
		e("http://oos2.com/api", false),
	)

	got := CollectOOSURLs(ents)
	if len(got) != 2 {
		t.Fatalf("expected 2 OOS URLs, got %d: %v", len(got), got)
	}
}

func TestCollectURLs_Deduplicates(t *testing.T) {
	ents := mkEntries(
		e("https://example.com", true),
		e("https://example.com", true),
	)

	got := CollectURLs(ents)
	if len(got) != 1 {
		t.Fatalf("expected 1 deduplicated URL, got %d: %v", len(got), got)
	}
}

func TestCollectURLs_Sorted(t *testing.T) {
	ents := mkEntries(
		e("https://z.com", true),
		e("https://a.com", true),
		e("https://m.com", true),
	)

	got := CollectURLs(ents)
	if got[0] != "https://a.com" || got[1] != "https://m.com" || got[2] != "https://z.com" {
		t.Fatalf("expected sorted results, got %v", got)
	}
}
