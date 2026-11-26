package scrapfly

import (
	"net/url"
	"testing"
)

func TestCloudBrowserShareCDP(t *testing.T) {
	client := &Client{key: "scp-test-key"}

	for _, share := range []bool{false, true} {
		endpoint, err := url.Parse(client.CloudBrowser(&CloudBrowserConfig{Session: "shared-shop", ShareCDP: share}))
		if err != nil {
			t.Fatal(err)
		}
		query := endpoint.Query()
		if query.Get("session") != "shared-shop" || query.Has("share_cdp") != share {
			t.Fatalf("unexpected join parameters for ShareCDP=%t: %v", share, query)
		}
		if share && query.Get("share_cdp") != "true" {
			t.Fatalf("unexpected share_cdp value: %q", query.Get("share_cdp"))
		}
	}
}
