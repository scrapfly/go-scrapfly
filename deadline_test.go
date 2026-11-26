package scrapfly

import (
	"net/http"
	"testing"
	"time"
)

// deadlineTransport records how long each request it sends has before its deadline.
type deadlineTransport struct {
	inner http.RoundTripper
	sent  []sentDeadline
}

type sentDeadline struct {
	set  bool
	left time.Duration
}

func (d *deadlineTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	sent := sentDeadline{}
	if deadline, ok := req.Context().Deadline(); ok {
		sent = sentDeadline{set: true, left: time.Until(deadline)}
	}
	d.sent = append(d.sent, sent)
	return d.inner.RoundTrip(req)
}

// sdkTimeout keeps the client the SDK builds; any other timeout is a client set with SetHTTPClient.
const sdkTimeout time.Duration = -1

func TestEachCallIsSentWithTheDeadlineOfItsAPI(t *testing.T) {
	scrape := func(c *Client) {
		_, _ = c.Scrape(&ScrapeConfig{URL: "https://web-scraping.dev/product/1"})
	}
	screenshot := func(c *Client) {
		_, _ = c.Screenshot(&ScreenshotConfig{URL: "https://web-scraping.dev/product/1"})
	}
	extraction := func(c *Client) {
		_, _ = c.Extract(&ExtractionConfig{Body: []byte("<h1>Box of Chocolate Candy</h1>"), ContentType: "text/html", ExtractionPrompt: "the product name"})
	}
	artifact := func(c *Client) {
		_, _ = c.CrawlArtifact("crawl-1", ArtifactTypeWARC)
	}
	prompt := func(c *Client) {
		_ = c.CrawlsPrompt([]string{"crawl-1"}, "the product name", nil, func(CrawlerPromptEvent) error { return nil })
	}
	crawlExtraction := func(c *Client) {
		_, _ = c.CrawlsExtract([]string{"crawl-1"}, "the product name", map[string]interface{}{"type": "object"}, nil)
	}

	cases := []struct {
		name string
		// timeout is the client's Timeout.
		timeout time.Duration
		call    func(*Client)
		// want is how long the request has; 0: no deadline.
		want time.Duration
	}{
		{"a scrape", sdkTimeout, scrape, 160 * time.Second},
		{"a screenshot", sdkTimeout, screenshot, 160 * time.Second},
		{"an extraction", sdkTimeout, extraction, 165 * time.Second},
		{"a crawl artifact", sdkTimeout, artifact, 300 * time.Second},
		{"a crawl prompt", sdkTimeout, prompt, 180 * time.Second},
		{"a crawl extraction", sdkTimeout, crawlExtraction, 180 * time.Second},
		{"a scrape under a shorter client timeout", time.Second, scrape, time.Second},
		{"an extraction under a shorter client timeout", time.Second, extraction, 165 * time.Second},
		{"an extraction under a longer client timeout", 10 * time.Minute, extraction, 10 * time.Minute},
		{"an extraction under no client timeout", 0, extraction, 0},
		{"a crawl artifact under a shorter client timeout", time.Second, artifact, 300 * time.Second},
		{"a crawl artifact under a longer client timeout", 10 * time.Minute, artifact, 10 * time.Minute},
		{"a crawl artifact under no client timeout", 0, artifact, 0},
		{"a crawl prompt under a longer client timeout", 10 * time.Minute, prompt, 10 * time.Minute},
		{"a crawl prompt under no client timeout", 0, prompt, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"refused"}`))
			})
			transport := &deadlineTransport{inner: client.HTTPClient().Transport}
			client.HTTPClient().Transport = transport
			if tc.timeout != sdkTimeout {
				client.SetHTTPClient(&http.Client{Timeout: tc.timeout, Transport: transport})
			}
			clientTimeout := client.HTTPClient().Timeout

			tc.call(client)

			if len(transport.sent) != 1 {
				t.Fatalf("%d requests went through the client's Transport, want 1", len(transport.sent))
			}
			sent := transport.sent[0]
			if tc.want == 0 && sent.set {
				t.Errorf("sent with %s left, want no deadline", sent.left)
			}
			if tc.want != 0 && (!sent.set || sent.left > tc.want || sent.left < tc.want-3*time.Second) {
				t.Errorf("sent with %s left (deadline set: %t), want %s", sent.left, sent.set, tc.want)
			}
			if client.HTTPClient().Timeout != clientTimeout {
				t.Errorf("the client's Timeout became %s, was %s", client.HTTPClient().Timeout, clientTimeout)
			}
		})
	}
}
