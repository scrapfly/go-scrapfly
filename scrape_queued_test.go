package scrapfly

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func scrapeAPI(t *testing.T, result string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"config":{"url":"https://web-scraping.dev/product/1"},"context":{},"uuid":"01","result":` + result + `}`))
	}))
	t.Cleanup(server.Close)
	client, err := NewWithHost("scp-test-key", server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestScrapeQueuedForAWebhookIsReturned(t *testing.T) {
	client := scrapeAPI(t, `{"status":"QUEUED","success":null,"status_code":null,"content":"","error":null}`)

	result, err := client.Scrape(&ScrapeConfig{URL: "https://web-scraping.dev/product/1", Webhook: "my-webhook"})
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	if result.Result.Status != "QUEUED" {
		t.Fatalf("status = %q, want QUEUED", result.Result.Status)
	}
}

func TestScrapeThatFailedIsStillAnError(t *testing.T) {
	client := scrapeAPI(t, `{"status":"DONE","success":false,"status_code":500,"content":"","error":null}`)

	if _, err := client.Scrape(&ScrapeConfig{URL: "https://web-scraping.dev/product/1"}); err == nil {
		t.Fatal("a failed scrape returned no error")
	}
}
