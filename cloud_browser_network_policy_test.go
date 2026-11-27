package scrapfly

import (
	"net/url"
	"testing"
)

func TestCloudBrowserNetworkPolicy(t *testing.T) {
	client := &Client{key: "scp-test-key"}
	for _, tc := range []struct {
		name   string
		policy *NetworkPolicy
		want   string
	}{
		{"unset", nil, ""},
		{"empty", &NetworkPolicy{}, ""},
		{"block", &NetworkPolicy{Block: []string{"httpbin.dev"}}, `{"block":["httpbin.dev"]}`},
		{"allow", &NetworkPolicy{Allow: []string{"web-scraping.dev"}}, `{"allow":["web-scraping.dev"]}`},
		{"mixed", &NetworkPolicy{Allow: []string{"web-scraping.dev", "2001:db8::1"}, Block: []string{"ads.web-scraping.dev"}}, `{"allow":["web-scraping.dev","2001:db8::1"],"block":["ads.web-scraping.dev"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := url.Parse(client.CloudBrowser(&CloudBrowserConfig{NetworkPolicy: tc.policy}))
			if err != nil {
				t.Fatal(err)
			}
			got := endpoint.Query()["network_policy"]
			if tc.want == "" {
				if len(got) > 0 {
					t.Fatalf("unexpected network_policy: %q", got)
				}
			} else if len(got) != 1 || got[0] != tc.want {
				t.Errorf("network_policy = %q, want %q", got, tc.want)
			}
		})
	}
}
