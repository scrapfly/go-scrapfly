package scrapfly

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestTransportErrorDoesNotPrintTheKey(t *testing.T) {
	client, err := NewWithHost("scp-live-0123456789abcdef", "http://127.0.0.1:1", false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.VerifyAPIKey()
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if strings.Contains(err.Error(), "0123456789abcdef") {
		t.Errorf("the error prints the key: %v", err)
	}
	if !strings.Contains(err.Error(), "key=REDACTED") {
		t.Errorf("the error lost the masked URL: %v", err)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Errorf("the error is no longer a *url.Error: %T", err)
	}
}

func TestPromptTransportErrorDoesNotPrintTheKey(t *testing.T) {
	client, err := NewWithHost("scp-live-0123456789abcdef", "http://127.0.0.1:1", false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.CrawlsPromptAnswer([]string{"0198aaaa"}, "q", nil)
	if err == nil || strings.Contains(err.Error(), "0123456789abcdef") {
		t.Errorf("expected a transport error without the key, got %v", err)
	}
}

func TestWithoutKey(t *testing.T) {
	for raw, want := range map[string]string{
		"https://api.scrapfly.io/scrape?key=scp-live-abc&url=https%3A%2F%2Fx.io": "https://api.scrapfly.io/scrape?key=REDACTED&url=https%3A%2F%2Fx.io",
		"https://api.scrapfly.io/account":                                        "https://api.scrapfly.io/account",
		"https://api.scrapfly.io/scrape?url=a":                                   "https://api.scrapfly.io/scrape?url=a",
		"https://api.scrapfly.io/x?key=%zz":                                      "https://api.scrapfly.io/x",
	} {
		if got := withoutKey(raw); got != want {
			t.Errorf("withoutKey(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestScreenshotDownloadErrorDoesNotPrintTheKey(t *testing.T) {
	_, err := (&Screenshot{URL: "http://127.0.0.1:1/scrape/screenshot/abc?key=scp-live-0123456789abcdef"}).Image()
	if err == nil || strings.Contains(err.Error(), "0123456789abcdef") {
		t.Errorf("expected a transport error without the key, got %v", err)
	}
}
