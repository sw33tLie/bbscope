package targets

import "testing"

func TestCollectDomains(t *testing.T) {
	// Note: isDomain matches anything with a dot that isn't a URL,
	// matching the original CLI behavior. This includes wildcards and IPs.
	ents := mkEntries(
		e("example.com", true),
		e("*.test.com", true),
		e("https://url.com", true),
		e("noperiod", true),
		e("oos.com", false),
	)

	got := CollectDomains(ents)
	if len(got) != 2 {
		t.Fatalf("expected 2 domains, got %d: %v", len(got), got)
	}
}

func TestCollectOOSDomains(t *testing.T) {
	ents := mkEntries(
		e("example.com", true),
		e("oos.com", false),
		e("*.oos2.com", false),
	)

	got := CollectOOSDomains(ents)
	if len(got) != 2 {
		t.Fatalf("expected 2 OOS domains, got %d: %v", len(got), got)
	}
}
