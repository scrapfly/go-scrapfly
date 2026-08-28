package scrapfly

import (
	"net/url"
	"strings"
	"testing"
)

// target_url is what lets the server pick a proxy that serves the destination.
// A session that omits it is routed blind, and an upstream provider refusing the
// target fails the whole run at CONNECT time, so the parameter has to survive
// onto the wire rather than merely being stored on the config.
func TestCloudBrowserSendsTargetURL(t *testing.T) {
	client := &Client{key: "scp-test-key"}

	raw := client.CloudBrowser(&CloudBrowserConfig{TargetURL: "https://web-scraping.dev/products"})

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("CloudBrowser returned an unparsable url %q: %v", raw, err)
	}
	if got := parsed.Query().Get("target_url"); got != "https://web-scraping.dev/products" {
		t.Fatalf("target_url = %q, want the declared target", got)
	}
}

func TestCloudBrowserOmitsTargetURLWhenUnset(t *testing.T) {
	client := &Client{key: "scp-test-key"}

	raw := client.CloudBrowser(&CloudBrowserConfig{})

	if strings.Contains(raw, "target_url") {
		t.Fatalf("target_url must not be sent when unset, got %q", raw)
	}
}

// The query string carries the api_key, so a target URL with its own query must
// be escaped rather than splicing extra parameters into the connect request.
func TestCloudBrowserEscapesTargetURLQuery(t *testing.T) {
	client := &Client{key: "scp-test-key"}
	target := "https://web-scraping.dev/products?q=1&proxy_pool=injected"

	parsed, err := url.Parse(client.CloudBrowser(&CloudBrowserConfig{TargetURL: target}))
	if err != nil {
		t.Fatalf("CloudBrowser returned an unparsable url: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("target_url"); got != target {
		t.Fatalf("target_url = %q, want %q", got, target)
	}
	if got := query.Get("proxy_pool"); got != "" {
		t.Fatalf("target_url query leaked into the connect params: proxy_pool = %q", got)
	}
}
