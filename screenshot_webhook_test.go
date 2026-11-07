package scrapfly

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const screenshotEnqueued = `{"success":true,"webhook_enqueued":true,"job_uuid":"job-1","log_uuid":"log-1","webhook_name":"my-webhook"}`

func screenshotAPI(t *testing.T, status int, contentType, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	client, err := NewWithHost("scp-test-key", server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func webhookScreenshot(client *Client) (*ScreenshotResult, error) {
	return client.Screenshot(&ScreenshotConfig{URL: "https://web-scraping.dev/product/1", Webhook: "my-webhook"})
}

func TestScreenshotEnqueuedForAWebhookReturnsItsJob(t *testing.T) {
	client := screenshotAPI(t, http.StatusCreated, "application/json; charset=utf-8", screenshotEnqueued)

	result, err := webhookScreenshot(client)
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if result.JobUUID != "job-1" {
		t.Fatalf("job = %q, want job-1", result.JobUUID)
	}
	if len(result.Image) != 0 {
		t.Fatalf("image of %d bytes, want none", len(result.Image))
	}
	if result.Metadata != (ScreenshotMetadata{}) {
		t.Fatalf("metadata = %+v, want none", result.Metadata)
	}
}

func TestSavingAScreenshotEnqueuedForAWebhookIsRefused(t *testing.T) {
	client := screenshotAPI(t, http.StatusCreated, "application/json", screenshotEnqueued)

	result, err := webhookScreenshot(client)
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if _, err := result.Save("shot", t.TempDir()); err == nil {
		t.Fatal("saved a screenshot with no image")
	}
}

func TestAnyOther201IsAnError(t *testing.T) {
	for _, body := range []string{`{"success":true}`, `{"webhook_enqueued":false,"job_uuid":"job-1"}`, `{"webhook_enqueued":true}`, `{"webhook_enqueued":true,"job_uuid":""}`, `not json`} {
		client := screenshotAPI(t, http.StatusCreated, "application/json", body)

		if result, err := webhookScreenshot(client); err == nil {
			t.Fatalf("%s: returned %+v, want an error", body, result)
		}
	}
}

func TestAJSONAnswerIsNeverAnImage(t *testing.T) {
	client := screenshotAPI(t, http.StatusOK, "application/json; charset=utf-8", `{"success":true}`)

	if result, err := webhookScreenshot(client); err == nil {
		t.Fatalf("returned %+v, want an error", result)
	}
}

func TestAnImageHasNoJob(t *testing.T) {
	client := screenshotAPI(t, http.StatusOK, "image/png", "\x89PNG")

	result, err := webhookScreenshot(client)
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if result.JobUUID != "" {
		t.Fatalf("job = %q, want none", result.JobUUID)
	}
	if string(result.Image) != "\x89PNG" || result.Metadata.ExtensionName != "png" {
		t.Fatalf("image %q, extension %q", result.Image, result.Metadata.ExtensionName)
	}
}
