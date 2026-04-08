package scrapfly

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ==============================================================================
// CrawlerConfig validation / serialization
// ==============================================================================

func TestCrawlerConfig_RequiredURL(t *testing.T) {
	config := &CrawlerConfig{}
	_, err := config.toJSONBody()
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
	if !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("expected ErrCrawlerConfig, got %v", err)
	}
}

func TestCrawlerConfig_MinimalConfigOnlyHasURL(t *testing.T) {
	config := &CrawlerConfig{URL: "https://example.com"}
	body, err := config.toJSONBody()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	_ = json.Unmarshal(body, &decoded)

	// URL is present.
	if decoded["url"] != "https://example.com" {
		t.Errorf("url not set: %v", decoded["url"])
	}
	// Every optional field is absent so the server applies its own defaults.
	forbidden := []string{
		"respect_robots_txt", "follow_internal_subdomains", "page_limit",
		"max_depth", "cache", "asp", "user_agent",
	}
	for _, key := range forbidden {
		if _, ok := decoded[key]; ok {
			t.Errorf("minimal config should not include %q, got %v", key, decoded[key])
		}
	}
}

func TestCrawlerConfig_AllFieldsSerialize(t *testing.T) {
	config := &CrawlerConfig{
		URL:                       "https://example.com",
		PageLimit:                 10,
		MaxDepth:                  3,
		MaxDuration:               600,
		MaxAPICredit:              5000,
		ExcludePaths:              []string{"/admin/*"},
		IgnoreBasePathRestriction: true,
		FollowExternalLinks:       true,
		AllowedExternalDomains:    []string{"cdn.example.com"},
		FollowInternalSubdomains:  BoolPtr(false),
		AllowedInternalSubdomains: []string{"blog.example.com"},
		Headers:                   map[string]string{"X-Custom": "v"},
		Delay:                     1000,
		UserAgent:                 "TestBot/1.0",
		MaxConcurrency:            5,
		RenderingDelay:            2000,
		UseSitemaps:               true,
		RespectRobotsTxt:          BoolPtr(false),
		IgnoreNoFollow:            true,
		Cache:                     true,
		CacheTTL:                  3600,
		CacheClear:                true,
		ContentFormats:            []CrawlerContentFormat{CrawlerFormatMarkdown, CrawlerFormatText},
		ASP:                       true,
		ProxyPool:                 "public_residential_pool",
		Country:                   "us",
		WebhookName:               "my-webhook",
		WebhookEvents:             []CrawlerWebhookEvent{WebhookCrawlerFinished, WebhookCrawlerURLFailed},
	}
	body, err := config.toJSONBody()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	_ = json.Unmarshal(body, &decoded)

	// Sanity-check a few of the fields — full field-by-field assertion
	// would duplicate the serialization logic.
	if decoded["page_limit"] != float64(10) {
		t.Errorf("page_limit: %v", decoded["page_limit"])
	}
	if decoded["follow_internal_subdomains"] != false {
		t.Errorf("follow_internal_subdomains: %v", decoded["follow_internal_subdomains"])
	}
	if decoded["respect_robots_txt"] != false {
		t.Errorf("respect_robots_txt: %v", decoded["respect_robots_txt"])
	}
	formats := decoded["content_formats"].([]interface{})
	if len(formats) != 2 || formats[0] != "markdown" {
		t.Errorf("content_formats: %v", formats)
	}
	events := decoded["webhook_events"].([]interface{})
	if len(events) != 2 || events[0] != "crawler_finished" {
		t.Errorf("webhook_events: %v", events)
	}
}

func TestCrawlerConfig_ExcludeAndIncludeAreMutuallyExclusive(t *testing.T) {
	config := &CrawlerConfig{
		URL:              "https://example.com",
		ExcludePaths:     []string{"/a/*"},
		IncludeOnlyPaths: []string{"/b/*"},
	}
	_, err := config.toJSONBody()
	if err == nil {
		t.Fatal("expected exclusive-fields error")
	}
	if !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("expected ErrCrawlerConfig, got %v", err)
	}
}

func TestCrawlerConfig_RenderingDelayBounds(t *testing.T) {
	cases := []struct {
		name  string
		value int
		valid bool
	}{
		{"negative", -1, false},
		{"zero (unset)", 0, true},
		{"min", 0, true},
		{"mid", 5000, true},
		{"max", 25000, true},
		{"over", 25001, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &CrawlerConfig{URL: "https://example.com", RenderingDelay: tc.value}
			_, err := config.toJSONBody()
			if tc.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestCrawlerConfig_MaxDurationBounds(t *testing.T) {
	// max_duration has tighter bounds: 15-10800 when set, 0 means unset.
	cases := []struct {
		name  string
		value int
		valid bool
	}{
		{"unset", 0, true},
		{"below min", 14, false},
		{"min", 15, true},
		{"max", 10800, true},
		{"above max", 10801, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &CrawlerConfig{URL: "https://example.com", MaxDuration: tc.value}
			_, err := config.toJSONBody()
			if tc.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tc.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestCrawlerConfig_ArraySizeLimits(t *testing.T) {
	exceed := make([]string, 101)
	for i := range exceed {
		exceed[i] = "/p"
	}
	config := &CrawlerConfig{URL: "https://example.com", ExcludePaths: exceed}
	_, err := config.toJSONBody()
	if err == nil {
		t.Fatal("expected size-limit error for exclude_paths")
	}
}

func TestCrawlerConfig_InvalidContentFormat(t *testing.T) {
	config := &CrawlerConfig{
		URL:            "https://example.com",
		ContentFormats: []CrawlerContentFormat{"pdf"},
	}
	_, err := config.toJSONBody()
	if err == nil {
		t.Fatal("expected error for invalid content format")
	}
}

func TestCrawlerConfig_InvalidWebhookEvent(t *testing.T) {
	config := &CrawlerConfig{
		URL:           "https://example.com",
		WebhookEvents: []CrawlerWebhookEvent{"crawl.started"},
	}
	_, err := config.toJSONBody()
	if err == nil {
		t.Fatal("expected error for invalid webhook event")
	}
}

func TestCrawlerConfig_AllEightValidWebhookEvents(t *testing.T) {
	config := &CrawlerConfig{
		URL: "https://example.com",
		WebhookEvents: []CrawlerWebhookEvent{
			WebhookCrawlerStarted,
			WebhookCrawlerURLVisited,
			WebhookCrawlerURLSkipped,
			WebhookCrawlerURLDiscovered,
			WebhookCrawlerURLFailed,
			WebhookCrawlerStopped,
			WebhookCrawlerCancelled,
			WebhookCrawlerFinished,
		},
	}
	if _, err := config.toJSONBody(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ==============================================================================
// parseCrawlerStatus strict parsing
// ==============================================================================

func TestParseCrawlerStatus_Happy(t *testing.T) {
	body := []byte(`{
		"crawler_uuid": "abc-123",
		"status": "DONE",
		"is_finished": true,
		"is_success": true,
		"state": {
			"urls_visited": 5,
			"urls_extracted": 20,
			"urls_failed": 1,
			"urls_skipped": 2,
			"urls_to_crawl": 12,
			"api_credit_used": 50,
			"duration": 30,
			"start_time": 1700000000,
			"stop_time": 1700000030,
			"stop_reason": "page_limit"
		}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if status.CrawlerUUID != "abc-123" {
		t.Errorf("uuid: %s", status.CrawlerUUID)
	}
	if !status.IsComplete() {
		t.Error("expected IsComplete() to be true")
	}
	if status.IsRunning() {
		t.Error("expected IsRunning() to be false for DONE")
	}
	// 5/20 * 100 = 25
	if status.ProgressPct() != 25 {
		t.Errorf("progress: %v", status.ProgressPct())
	}
	if !status.State.HasStarted() {
		t.Error("expected HasStarted()")
	}
}

func TestParseCrawlerStatus_PendingNullableFields(t *testing.T) {
	// Matches the actual server response for a PENDING crawl, where the time
	// fields are explicitly null.
	body := []byte(`{
		"crawler_uuid": "pending-1",
		"status": "PENDING",
		"is_finished": false,
		"is_success": false,
		"state": {
			"urls_visited": 0,
			"urls_extracted": 0,
			"urls_failed": 0,
			"urls_skipped": 0,
			"urls_to_crawl": 0,
			"api_credit_used": 0,
			"duration": 0,
			"start_time": null,
			"stop_time": null,
			"stop_reason": null
		}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if status.State.HasStarted() {
		t.Error("HasStarted should be false for PENDING")
	}
	if status.State.HasStopped() {
		t.Error("HasStopped should be false for PENDING")
	}
	if status.State.StopReason != nil {
		t.Errorf("StopReason should be nil for PENDING, got %v", *status.State.StopReason)
	}
	if !status.IsRunning() {
		t.Error("IsRunning should be true for PENDING")
	}
	// IsFailed must not be true just because is_success=false — it only
	// counts as failed when status=DONE. This was an actual gotcha during
	// the Python/TS ports.
	if status.IsFailed() {
		t.Error("IsFailed should NOT be true for PENDING even when is_success=false")
	}
}

func TestParseCrawlerStatus_MissingRequiredField(t *testing.T) {
	body := []byte(`{"crawler_uuid": "x", "status": "DONE", "is_finished": true}`)
	_, err := parseCrawlerStatus(body)
	if err == nil {
		t.Fatal("expected error for missing state field")
	}
	if !strings.Contains(err.Error(), "state") {
		t.Errorf("error should mention missing field: %v", err)
	}
}

func TestParseCrawlerStatus_MissingStateCounter(t *testing.T) {
	body := []byte(`{
		"crawler_uuid": "x", "status": "DONE", "is_finished": true,
		"state": {"urls_visited": 1}
	}`)
	_, err := parseCrawlerStatus(body)
	if err == nil {
		t.Fatal("expected error for missing state counter")
	}
}

// ==============================================================================
// parseCrawlerURLs streaming text parsing
// ==============================================================================

func TestParseCrawlerURLs_Visited(t *testing.T) {
	body := "https://example.com/a\nhttps://example.com/b\nhttps://example.com/c\n"
	urls := parseCrawlerURLs(body, "visited", 1, 100)
	if len(urls.URLs) != 3 {
		t.Fatalf("expected 3 urls, got %d", len(urls.URLs))
	}
	if urls.URLs[0].URL != "https://example.com/a" {
		t.Errorf("first url: %s", urls.URLs[0].URL)
	}
	if urls.URLs[0].Status != "visited" {
		t.Errorf("status: %s", urls.URLs[0].Status)
	}
	if urls.URLs[0].Reason != "" {
		t.Errorf("reason should be empty for visited, got %s", urls.URLs[0].Reason)
	}
}

func TestParseCrawlerURLs_FailedWithReason(t *testing.T) {
	body := "https://example.com/404,page_limit\nhttps://example.com/500,crawler_error\n"
	urls := parseCrawlerURLs(body, "failed", 1, 100)
	if len(urls.URLs) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(urls.URLs))
	}
	if urls.URLs[0].Reason != "page_limit" {
		t.Errorf("reason: %s", urls.URLs[0].Reason)
	}
	if urls.URLs[1].URL != "https://example.com/500" {
		t.Errorf("second url: %s", urls.URLs[1].URL)
	}
}

func TestParseCrawlerURLs_BlankLinesIgnored(t *testing.T) {
	body := "\nhttps://example.com/a\n\n\nhttps://example.com/b\n\n"
	urls := parseCrawlerURLs(body, "visited", 1, 100)
	if len(urls.URLs) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(urls.URLs))
	}
}

func TestParseCrawlerURLs_CRLFTrimmed(t *testing.T) {
	body := "https://example.com/a  \r\n  https://example.com/b\r\n"
	urls := parseCrawlerURLs(body, "visited", 1, 100)
	if len(urls.URLs) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(urls.URLs))
	}
	if urls.URLs[0].URL != "https://example.com/a" {
		t.Errorf("expected trimmed url, got %q", urls.URLs[0].URL)
	}
}

func TestParseCrawlerURLs_EmptyBody(t *testing.T) {
	urls := parseCrawlerURLs("", "visited", 2, 50)
	if len(urls.URLs) != 0 {
		t.Errorf("expected 0 urls for empty body, got %d", len(urls.URLs))
	}
	if urls.Page != 2 || urls.PerPage != 50 {
		t.Errorf("page/per_page should echo caller inputs: page=%d per_page=%d", urls.Page, urls.PerPage)
	}
}

// ==============================================================================
// parseCrawlerContents strict parsing
// ==============================================================================

func TestParseCrawlerContents_Happy(t *testing.T) {
	body := []byte(`{
		"contents": {
			"https://example.com/a": {"markdown": "# A"},
			"https://example.com/b": {"markdown": "# B"}
		},
		"links": {"next": null, "prev": null}
	}`)
	contents, err := parseCrawlerContents(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents.Contents) != 2 {
		t.Errorf("expected 2 urls, got %d", len(contents.Contents))
	}
}

func TestParseCrawlerContents_MissingContents(t *testing.T) {
	body := []byte(`{"links": {}}`)
	_, err := parseCrawlerContents(body)
	if err == nil {
		t.Fatal("expected error for missing contents")
	}
}

// ==============================================================================
// End-to-end tests via httptest
// ==============================================================================

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewWithHost("__API_KEY__", server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestClient_StartCrawl_POSTsJSONBody(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/crawl" {
			t.Errorf("expected /crawl, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "__API_KEY__" {
			t.Errorf("key missing in query")
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(bodyBytes, &body)
		if body["url"] != "https://web-scraping.dev/products" {
			t.Errorf("url not set in body: %v", body["url"])
		}
		if body["page_limit"] != float64(5) {
			t.Errorf("page_limit: %v", body["page_limit"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crawler_uuid": "abc-123", "status": "PENDING"}`))
	})

	resp, err := client.StartCrawl(&CrawlerConfig{
		URL:       "https://web-scraping.dev/products",
		PageLimit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.CrawlerUUID != "abc-123" {
		t.Errorf("uuid: %s", resp.CrawlerUUID)
	}
}

func TestClient_StartCrawl_401ReturnsAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error_id": "x", "http_code": 401, "message": "Invalid API key"}`))
	})
	_, err := client.StartCrawl(&CrawlerConfig{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("expected *APIError, got %T: %v", err, err)
	}
}

func TestClient_StartCrawl_CrawlerResourceErrorWraps(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error_id": "x", "http_code": 422, "code": "ERR::CRAWLER::HIGH_FAILURE_RATE", "message": "high failure rate"}`))
	})
	_, err := client.StartCrawl(&CrawlerConfig{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Errorf("expected ErrCrawlerFailed wrap, got %v", err)
	}
}

func TestClient_CrawlStatus_ParsesResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/abc-123/status") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"crawler_uuid": "abc-123", "status": "RUNNING",
			"is_finished": false, "is_success": null,
			"state": {
				"urls_visited": 5, "urls_extracted": 20, "urls_failed": 1,
				"urls_skipped": 2, "urls_to_crawl": 12, "api_credit_used": 50,
				"duration": 30, "start_time": 1700000000, "stop_time": null, "stop_reason": null
			}
		}`))
	})
	status, err := client.CrawlStatus("abc-123")
	if err != nil {
		t.Fatal(err)
	}
	if !status.IsRunning() {
		t.Error("should be running")
	}
	if status.State.HasStarted() == false {
		t.Error("should have started")
	}
	if status.State.HasStopped() == true {
		t.Error("should not have stopped")
	}
}

func TestClient_CrawlURLs_StreamingText(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/abc-123/urls") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("status") != "visited" {
			t.Errorf("status query: %s", r.URL.Query().Get("status"))
		}
		if r.URL.Query().Get("page") != "2" {
			t.Errorf("page query: %s", r.URL.Query().Get("page"))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("https://example.com/a\nhttps://example.com/b\n"))
	})
	urls, err := client.CrawlURLs("abc-123", &CrawlURLsOptions{
		Status:  "visited",
		Page:    2,
		PerPage: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(urls.URLs) != 2 {
		t.Fatalf("expected 2 urls, got %d", len(urls.URLs))
	}
	if urls.Page != 2 {
		t.Errorf("page echo: %d", urls.Page)
	}
}

func TestClient_CrawlURLs_FailedWithReason(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("https://example.com/404,page_limit\n"))
	})
	urls, err := client.CrawlURLs("abc-123", &CrawlURLsOptions{Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(urls.URLs) != 1 {
		t.Fatalf("expected 1 url, got %d", len(urls.URLs))
	}
	if urls.URLs[0].Reason != "page_limit" {
		t.Errorf("reason: %s", urls.URLs[0].Reason)
	}
}

func TestClient_CrawlURLs_JSONOnSuccessReturnsFormatError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Unexpected: server sends JSON on a 200 for this text endpoint.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"urls": []}`))
	})
	_, err := client.CrawlURLs("abc-123", nil)
	if err == nil {
		t.Fatal("expected format error")
	}
	if !errors.Is(err, ErrUnexpectedResponseFormat) {
		t.Errorf("expected ErrUnexpectedResponseFormat, got %v", err)
	}
}

func TestClient_CrawlContentsJSON(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Server expects `formats` (plural), not `format`.
		if r.URL.Query().Get("formats") != "markdown" {
			t.Errorf("formats param: %s", r.URL.Query().Get("formats"))
		}
		if r.URL.Query().Get("plain") != "" {
			t.Errorf("plain should not be set in JSON mode")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contents": {"https://example.com/p1": {"markdown": "# Page 1"}},
			"links": {"next": null, "prev": null}
		}`))
	})
	result, err := client.CrawlContentsJSON("abc-123", CrawlerFormatMarkdown, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 url, got %d", len(result.Contents))
	}
}

func TestClient_CrawlContentsPlain(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("plain") != "true" {
			t.Error("plain should be true")
		}
		if r.URL.Query().Get("url") != "https://example.com/p1" {
			t.Error("url should be set")
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Page 1 markdown content"))
	})
	result, err := client.CrawlContentsPlain("abc-123", "https://example.com/p1", CrawlerFormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if result != "# Page 1 markdown content" {
		t.Errorf("plain body: %q", result)
	}
}

func TestClient_CrawlContentsPlain_RequiresURL(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should not be sent")
	})
	_, err := client.CrawlContentsPlain("abc-123", "", CrawlerFormatMarkdown)
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestClient_CrawlContentsBatch_ParsesMultipart(t *testing.T) {
	boundary := "mp-test-boundary"
	multipartBody := strings.Join([]string{
		"--" + boundary,
		"Content-Type: text/markdown",
		"Content-Location: https://example.com/page1",
		"",
		"# Page 1",
		"--" + boundary,
		"Content-Type: text/markdown",
		"Content-Location: https://example.com/page2",
		"",
		"# Page 2",
		"--" + boundary + "--",
		"",
	}, "\r\n")

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("formats") != "markdown" {
			t.Errorf("formats: %s", r.URL.Query().Get("formats"))
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		if string(bodyBytes) != "https://example.com/page1\nhttps://example.com/page2" {
			t.Errorf("body: %q", string(bodyBytes))
		}
		w.Header().Set("Content-Type", "multipart/related; boundary="+boundary)
		_, _ = w.Write([]byte(multipartBody))
	})
	result, err := client.CrawlContentsBatch(
		"abc-123",
		[]string{"https://example.com/page1", "https://example.com/page2"},
		[]CrawlerContentFormat{CrawlerFormatMarkdown},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result["https://example.com/page1"]["markdown"] != "# Page 1" {
		t.Errorf("page1: %q", result["https://example.com/page1"]["markdown"])
	}
	if result["https://example.com/page2"]["markdown"] != "# Page 2" {
		t.Errorf("page2: %q", result["https://example.com/page2"]["markdown"])
	}
}

func TestClient_CrawlContentsBatch_EmptyURLsReturnsError(t *testing.T) {
	client := newTestClient(t, nil)
	_, err := client.CrawlContentsBatch("abc-123", []string{}, []CrawlerContentFormat{CrawlerFormatMarkdown})
	if err == nil {
		t.Fatal("expected error for empty URLs")
	}
}

func TestClient_CrawlContentsBatch_Over100URLsReturnsError(t *testing.T) {
	client := newTestClient(t, nil)
	urls := make([]string, 101)
	for i := range urls {
		urls[i] = "https://example.com/p"
	}
	_, err := client.CrawlContentsBatch("abc-123", urls, []CrawlerContentFormat{CrawlerFormatMarkdown})
	if err == nil {
		t.Fatal("expected error for >100 urls")
	}
}

func TestClient_CrawlArtifact_WARCBytes(t *testing.T) {
	warcBytes := []byte{0x1f, 0x8b, 0x08, 0x00, 0xde, 0xad, 0xbe, 0xef}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "warc" {
			t.Errorf("type: %s", r.URL.Query().Get("type"))
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(warcBytes)
	})
	artifact, err := client.CrawlArtifact("abc-123", ArtifactTypeWARC)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Type != ArtifactTypeWARC {
		t.Errorf("type: %s", artifact.Type)
	}
	if artifact.Len() != 8 {
		t.Errorf("data length: %d", artifact.Len())
	}
}

func TestClient_CrawlArtifact_HARJSON(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "har" {
			t.Errorf("type: %s", r.URL.Query().Get("type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"log": {"version": "1.2", "entries": []}}`))
	})
	artifact, err := client.CrawlArtifact("abc-123", ArtifactTypeHAR)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Type != ArtifactTypeHAR {
		t.Errorf("type: %s", artifact.Type)
	}
	if artifact.Len() == 0 {
		t.Error("expected non-empty HAR")
	}
}

func TestClient_CrawlCancel_Success(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/abc-123/cancel") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := client.CrawlCancel("abc-123"); err != nil {
		t.Errorf("CrawlCancel: %v", err)
	}
}

func TestClient_CrawlCancel_404Error(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error_id": "x", "http_code": 404, "code": "ERR::CRAWLER::NOT_FOUND", "message": "Crawl not found"}`))
	})
	err := client.CrawlCancel("abc-123")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Errorf("expected ErrCrawlerFailed wrap, got %v", err)
	}
}

func TestCrawl_Cancel_DelegatesToClient(t *testing.T) {
	called := false
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/crawl" {
			_, _ = w.Write([]byte(`{"crawler_uuid": "abc", "status": "PENDING"}`))
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/abc/cancel") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com"})
	if err := crawl.Start(); err != nil {
		t.Fatal(err)
	}
	if err := crawl.Cancel(); err != nil {
		t.Errorf("Cancel: %v", err)
	}
	if !called {
		t.Error("cancel endpoint not invoked")
	}
}

func TestCrawl_Cancel_NotStartedReturnsError(t *testing.T) {
	client, _ := New("__API_KEY__")
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com"})
	if err := crawl.Cancel(); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("expected ErrCrawlerNotStarted, got %v", err)
	}
}

func TestClient_CrawlArtifact_ErrorEnvelope(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error_id": "x", "http_code": 404, "code": "ERR::CRAWLER::NOT_FOUND", "message": "Crawl not found"}`))
	})
	_, err := client.CrawlArtifact("abc-123", ArtifactTypeWARC)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Errorf("expected ErrCrawlerFailed wrap, got %v", err)
	}
}

// ==============================================================================
// High-level Crawl helper
// ==============================================================================

func TestCrawl_StartOnceThenError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"crawler_uuid": "abc", "status": "PENDING"}`))
	})
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com"})
	if crawl.Started() {
		t.Error("should not be started yet")
	}
	if err := crawl.Start(); err != nil {
		t.Fatal(err)
	}
	if !crawl.Started() {
		t.Error("should be started")
	}
	if crawl.UUID() != "abc" {
		t.Errorf("uuid: %s", crawl.UUID())
	}
	// Second Start() is an error.
	if err := crawl.Start(); !errors.Is(err, ErrCrawlerAlreadyStarted) {
		t.Errorf("expected ErrCrawlerAlreadyStarted, got %v", err)
	}
}

func TestCrawl_MethodsErrorBeforeStart(t *testing.T) {
	client, _ := New("__API_KEY__")
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com"})
	if _, err := crawl.Status(true); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("Status: %v", err)
	}
	if _, err := crawl.URLs(nil); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("URLs: %v", err)
	}
	if _, err := crawl.WARC(); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("WARC: %v", err)
	}
}

func TestCrawl_Wait_TerminalSuccess(t *testing.T) {
	pollCount := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		pollCount++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/crawl" {
			_, _ = w.Write([]byte(`{"crawler_uuid": "abc", "status": "PENDING"}`))
			return
		}
		// /crawl/abc/status — pretend the first poll returns RUNNING and the second returns DONE.
		if pollCount >= 3 {
			_, _ = w.Write([]byte(`{
				"crawler_uuid": "abc", "status": "DONE",
				"is_finished": true, "is_success": true,
				"state": {
					"urls_visited": 5, "urls_extracted": 5, "urls_failed": 0,
					"urls_skipped": 0, "urls_to_crawl": 0, "api_credit_used": 10,
					"duration": 5, "start_time": 1700000000, "stop_time": 1700000005,
					"stop_reason": "no_more_urls"
				}
			}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"crawler_uuid": "abc", "status": "RUNNING",
			"is_finished": false, "is_success": null,
			"state": {
				"urls_visited": 1, "urls_extracted": 5, "urls_failed": 0,
				"urls_skipped": 0, "urls_to_crawl": 4, "api_credit_used": 1,
				"duration": 1, "start_time": 1700000000, "stop_time": null, "stop_reason": null
			}
		}`))
	})
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com"})
	if err := crawl.Start(); err != nil {
		t.Fatal(err)
	}
	err := crawl.Wait(&WaitOptions{PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("Wait should succeed: %v", err)
	}
	status, _ := crawl.Status(false)
	if !status.IsComplete() {
		t.Error("status should be complete after Wait returns nil")
	}
}

func TestCrawl_Wait_TimeoutReturnsError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/crawl" {
			_, _ = w.Write([]byte(`{"crawler_uuid": "abc", "status": "PENDING"}`))
			return
		}
		// Always return RUNNING — Wait should time out.
		_, _ = w.Write([]byte(`{
			"crawler_uuid": "abc", "status": "RUNNING",
			"is_finished": false, "is_success": null,
			"state": {
				"urls_visited": 0, "urls_extracted": 0, "urls_failed": 0,
				"urls_skipped": 0, "urls_to_crawl": 0, "api_credit_used": 0,
				"duration": 0, "start_time": null, "stop_time": null, "stop_reason": null
			}
		}`))
	})
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com"})
	_ = crawl.Start()
	err := crawl.Wait(&WaitOptions{
		PollInterval: 20 * time.Millisecond,
		MaxWait:      50 * time.Millisecond,
	})
	if !errors.Is(err, ErrCrawlerTimeout) {
		t.Errorf("expected ErrCrawlerTimeout, got %v", err)
	}
}

// ==============================================================================
// Webhook event detection
// ==============================================================================

func TestDetectCrawlerWebhookEvent_Lifecycle(t *testing.T) {
	body := []byte(`{"event": "crawler_started", "payload": {}}`)
	event, err := DetectCrawlerWebhookEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if event != WebhookCrawlerStarted {
		t.Errorf("event: %s", event)
	}
}

func TestDetectCrawlerWebhookEvent_UnknownEventReturnsError(t *testing.T) {
	body := []byte(`{"event": "crawl.started", "payload": {}}`)
	_, err := DetectCrawlerWebhookEvent(body)
	if err == nil {
		t.Fatal("expected error for unknown event")
	}
}

func TestDetectCrawlerWebhookEvent_MissingEvent(t *testing.T) {
	body := []byte(`{"payload": {}}`)
	_, err := DetectCrawlerWebhookEvent(body)
	if err == nil {
		t.Fatal("expected error for missing event")
	}
}
