package scrapfly

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func batchWebhookPart(t *testing.T, id string, scrape map[string]any, status int) *Client {
	t.Helper()
	return newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		part, err := json.Marshal(map[string]any{"correlation_id": id, "webhook_name": "my-webhook", "job_uuid": "01", "enqueued": true, "result": scrape})
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "multipart/mixed; boundary=b0und")
		fmt.Fprintf(w, "--b0und\r\nContent-Type: application/json\r\nX-Scrapfly-Correlation-Id: %s\r\nX-Scrapfly-Scrape-Status: %d\r\nContent-Length: %d\r\n\r\n%s\r\n--b0und--\r\n", id, status, len(part), part)
	})
}

func TestScrapeBatchReadsTheEnqueueAckOfAWebhook(t *testing.T) {
	scrape := map[string]any{
		"config":  map[string]any{"url": "https://web-scraping.dev/product/1", "correlation_id": "hook"},
		"context": map[string]any{},
		"uuid":    "01",
		"result":  map[string]any{"status": "QUEUED", "success": nil, "status_code": nil, "content": ""},
	}
	client := batchWebhookPart(t, "hook", scrape, http.StatusAccepted)

	results, err := client.ScrapeBatch([]*ScrapeConfig{{URL: "https://web-scraping.dev/product/1", CorrelationID: "hook", Webhook: "my-webhook"}})
	if err != nil {
		t.Fatal(err)
	}
	part := <-results
	if part.Err != nil {
		t.Fatalf("part: %v", part.Err)
	}
	if part.Result.Result.Status != "QUEUED" || part.Result.UUID != "01" {
		t.Fatalf("status %q, uuid %q", part.Result.Result.Status, part.Result.UUID)
	}
}

func TestScrapeBatchKeepsTheErrorOfAFailedEnqueue(t *testing.T) {
	scrape := map[string]any{
		"config":  map[string]any{"url": "https://web-scraping.dev/product/1", "correlation_id": "hook"},
		"context": map[string]any{},
		"uuid":    "01",
		"result":  map[string]any{"status": "DONE", "success": false, "content": "", "error": map[string]any{"code": "ERR::WEBHOOK::QUEUE_FULL", "message": "full"}},
	}
	client := batchWebhookPart(t, "hook", scrape, http.StatusTooManyRequests)

	results, err := client.ScrapeBatch([]*ScrapeConfig{{URL: "https://web-scraping.dev/product/1", CorrelationID: "hook", Webhook: "my-webhook"}})
	if err != nil {
		t.Fatal(err)
	}
	part := <-results
	if part.Err != nil {
		t.Fatalf("part: %v", part.Err)
	}
	if part.Result.Result.Error == nil || part.Result.Result.Error.Code != "ERR::WEBHOOK::QUEUE_FULL" {
		t.Fatalf("the error is lost: %+v", part.Result.Result.Error)
	}
}
