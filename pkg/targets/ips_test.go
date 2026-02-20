package targets

import "testing"

func TestCollectIPs(t *testing.T) {
	ents := mkEntries(
		e("1.2.3.4", true),
		e("https://5.6.7.8/path", true),
		e("example.com", true),
		e("9.9.9.9", false),
	)

	got := CollectIPs(ents)
	if len(got) != 2 {
		t.Fatalf("expected 2 IPs, got %d: %v", len(got), got)
	}
}

func TestCollectOOSIPs(t *testing.T) {
	ents := mkEntries(
		e("1.2.3.4", true),
		e("9.9.9.9", false),
	)

	got := CollectOOSIPs(ents)
	if len(got) != 1 {
		t.Fatalf("expected 1 OOS IP, got %d: %v", len(got), got)
	}
	if got[0] != "9.9.9.9" {
		t.Fatalf("expected 9.9.9.9, got %s", got[0])
	}
}

func TestCollectIPs_ExtractsFromURL(t *testing.T) {
	ents := mkEntries(
		e("https://10.0.0.1:8080/api", true),
	)

	got := CollectIPs(ents)
	if len(got) != 1 || got[0] != "10.0.0.1" {
		t.Fatalf("expected [10.0.0.1], got %v", got)
	}
}
