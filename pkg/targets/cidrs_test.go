package targets

import "testing"

func TestCollectCIDRs(t *testing.T) {
	ents := mkEntries(
		e("10.0.0.0/8", true),
		e("1.1.1.1-2.2.2.2", true),
		e("example.com", true),
		e("192.168.0.0/16", false),
	)

	got := CollectCIDRs(ents)
	if len(got) != 2 {
		t.Fatalf("expected 2 CIDRs, got %d: %v", len(got), got)
	}
}

func TestCollectOOSCIDRs(t *testing.T) {
	ents := mkEntries(
		e("10.0.0.0/8", true),
		e("192.168.0.0/16", false),
	)

	got := CollectOOSCIDRs(ents)
	if len(got) != 1 {
		t.Fatalf("expected 1 OOS CIDR, got %d: %v", len(got), got)
	}
}
