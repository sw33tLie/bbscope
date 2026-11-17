package bugcrowd

import (
	"net/http/cookiejar"
	"testing"
	"time"
)

func TestLoginHTTPClientHasBoundedRequestsAndNoRetries(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := newLoginHTTPClient(jar)
	if client.HTTPClient.Timeout != 30*time.Second {
		t.Fatalf("login timeout = %s, want 30s", client.HTTPClient.Timeout)
	}
	if client.RetryMax != 0 {
		t.Fatalf("login retry max = %d, want 0", client.RetryMax)
	}
	if client.HTTPClient.Jar != jar {
		t.Fatal("login client did not preserve the session cookie jar")
	}
}

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
