//go:build integration

// Integration tests for the Crawler API against a real Scrapfly server.
//
// These tests are gated behind the `integration` build tag so they don't run
// during a normal `go test` invocation. To run them, point at a live cluster
// and explicitly opt in:
//
//	export SCRAPFLY_API_KEY=scp-live-d8ac176c2f9d48b993b58675bdf71615
//	export SCRAPFLY_API_HOST=https://api.scrapfly.home
//	go test -tags=integration -timeout=300s -run TestIntegrationCrawler ./...
//
// The `api.scrapfly.home` host uses a self-signed TLS certificate, so this
// test file calls NewWithHost(key, host, false) — the third arg disables SSL
// verification for the duration of the test run. Do NOT use verifySSL=false
// against production hosts.

package scrapfly

import (
	"os"
	"testing"
	"time"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("SCRAPFLY_API_KEY")
	if key == "" {
		t.Skip("SCRAPFLY_API_KEY not set — skipping integration test")
	}
	host := os.Getenv("SCRAPFLY_API_HOST")
	if host == "" {
		host = "https://api.scrapfly.home"
	}
	// verifySSL=false because api.scrapfly.home uses a self-signed cert in dev.
	client, err := NewWithHost(key, host, false)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// startSmallCrawl schedules a 2-URL crawl and waits for it to finish.
// Used as a fixture by several of the integration tests below.
func startSmallCrawl(t *testing.T, client *Client) *Crawl {
	t.Helper()
	crawl := NewCrawl(client, &CrawlerConfig{
		URL:            "https://web-scraping.dev/products",
		PageLimit:      2,
		MaxDuration:    30,
		ContentFormats: []CrawlerContentFormat{CrawlerFormatMarkdown},
	})
	if err := crawl.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := crawl.Wait(&WaitOptions{
		PollInterval: 2 * time.Second,
		MaxWait:      120 * time.Second,
	}); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return crawl
}

func TestIntegrationCrawlerLifecycle(t *testing.T) {
	client := integrationClient(t)
	crawl := startSmallCrawl(t, client)

	status, err := crawl.Status(true)
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsComplete() {
		t.Errorf("expected IsComplete(), got status=%s is_success=%v", status.Status, status.IsSuccess)
	}
	if !status.State.HasStarted() {
		t.Error("expected HasStarted() to be true for DONE crawl")
	}
	if !status.State.HasStopped() {
		t.Error("expected HasStopped() to be true for DONE crawl")
	}
	if status.State.URLsVisited < 1 {
		t.Errorf("expected at least 1 visited URL, got %d", status.State.URLsVisited)
	}
	if status.State.StopReason == nil {
		t.Error("expected non-nil StopReason for DONE crawl")
	}
	stopReason := "<nil>"
	if status.State.StopReason != nil {
		stopReason = *status.State.StopReason
	}
	t.Logf("crawl %s: visited=%d duration=%ds stop_reason=%s",
		crawl.UUID(), status.State.URLsVisited, status.State.Duration, stopReason)
}

func TestIntegrationCrawlContentsJSON(t *testing.T) {
	client := integrationClient(t)
	crawl := startSmallCrawl(t, client)

	result, err := crawl.Contents(CrawlerFormatMarkdown, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) == 0 {
		t.Error("expected at least one crawled URL with content")
	}
	for url, formats := range result.Contents {
		if formats["markdown"] == "" {
			t.Errorf("expected non-empty markdown for %s", url)
		}
	}
}

func TestIntegrationCrawlContentsPlain(t *testing.T) {
	client := integrationClient(t)
	crawl := startSmallCrawl(t, client)

	// The seed URL is always visited, so we can read it back without having
	// to first list all visited URLs.
	seedURL := "https://web-scraping.dev/products"
	md, err := crawl.ReadString(seedURL, CrawlerFormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if md == "" {
		t.Errorf("expected non-empty markdown body for seed URL %s", seedURL)
	}
	t.Logf("plain markdown length: %d chars", len(md))
}

func TestIntegrationCrawlArtifactWARC(t *testing.T) {
	client := integrationClient(t)
	crawl := startSmallCrawl(t, client)

	artifact, err := crawl.WARC()
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Type != ArtifactTypeWARC {
		t.Errorf("type: %s", artifact.Type)
	}
	if artifact.Len() == 0 {
		t.Error("expected non-empty WARC artifact bytes")
	}
	t.Logf("warc artifact: %d bytes", artifact.Len())
}

func TestIntegrationCrawlCancel(t *testing.T) {
	client := integrationClient(t)

	// Schedule a long-running crawl so we can cancel it before it finishes.
	crawl := NewCrawl(client, &CrawlerConfig{
		URL:         "https://web-scraping.dev/products",
		PageLimit:   1000,
		MaxDuration: 600,
	})
	if err := crawl.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Logf("scheduled crawl %s", crawl.UUID())

	if err := crawl.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// Give the engine a moment to flush the cancellation, then verify the
	// terminal state matches expectations.
	time.Sleep(2 * time.Second)
	status, err := crawl.Status(true)
	if err != nil {
		t.Fatalf("Status after Cancel: %v", err)
	}
	if !status.IsCancelled() {
		t.Errorf("expected status=CANCELLED, got %s", status.Status)
	}
	if status.State.StopReason == nil || *status.State.StopReason != "user_cancelled" {
		stopReason := "<nil>"
		if status.State.StopReason != nil {
			stopReason = *status.State.StopReason
		}
		t.Errorf("expected stop_reason=user_cancelled, got %s", stopReason)
	}
	t.Logf("crawl %s cancelled successfully (status=%s stop_reason=%s)",
		crawl.UUID(), status.Status, *status.State.StopReason)
}

// TestIntegrationCrawlCancelIdempotent verifies that cancelling an already-
// finished crawl is a no-op (returns success without error).
func TestIntegrationCrawlCancelIdempotent(t *testing.T) {
	client := integrationClient(t)
	crawl := startSmallCrawl(t, client) // wait for it to finish naturally

	// Cancelling a finished crawl should not error — the endpoint is idempotent.
	if err := crawl.Cancel(); err != nil {
		t.Errorf("expected idempotent cancel on finished crawl, got error: %v", err)
	}
}

func TestIntegrationCrawlURLs(t *testing.T) {
	client := integrationClient(t)
	crawl := startSmallCrawl(t, client)

	// The server may return an empty body for very short crawls (known
	// separate server-side issue). The SDK text-parsing path works regardless
	// — a zero-record response is still a valid parse.
	urls, err := crawl.URLs(&CrawlURLsOptions{
		Status:  "visited",
		PerPage: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if urls.Page != 1 {
		t.Errorf("expected page=1, got %d", urls.Page)
	}
	if urls.PerPage != 50 {
		t.Errorf("expected per_page=50, got %d", urls.PerPage)
	}
	for _, entry := range urls.URLs {
		if entry.URL == "" {
			t.Error("empty URL in record")
		}
		if entry.Status != "visited" {
			t.Errorf("expected visited, got %s", entry.Status)
		}
	}
	t.Logf("crawl urls: %d records", len(urls.URLs))
}
