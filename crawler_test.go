package scrapfly

import (
	"encoding/json"
	"errors"
	"fmt"
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
	// "asp" is the anti-bot wire key and "unblocker" is its SDK-facing name:
	// neither may appear when the caller set nothing — the first would force
	// the feature on, the second is a key the API does not know.
	forbidden := []string{
		"respect_robots_txt", "follow_internal_subdomains", "page_limit",
		"max_depth", "cache", "asp", "unblocker", "user_agent",
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

func TestCrawlerConfig_AllValidWebhookEvents(t *testing.T) {
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
			WebhookCrawlerSearchReady,
			WebhookCrawlerSearchFailed,
			WebhookCrawlerUpdated,
		},
	}
	if _, err := config.toJSONBody(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCrawlerConfig_SearchSerializes(t *testing.T) {
	body, err := (&CrawlerConfig{URL: "https://example.com", Search: true}).toJSONBody()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["search"] != true {
		t.Errorf("search not on the wire: %v", decoded["search"])
	}
}

func TestCrawlerConfig_SearchOmittedWhenOff(t *testing.T) {
	// Unset means server default: never emit a field to send its default.
	body, err := (&CrawlerConfig{URL: "https://example.com"}).toJSONBody()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["search"]; present {
		t.Error("search must be absent when not requested")
	}
}

func TestCrawlerWebhookEvent_SearchEventsAreValid(t *testing.T) {
	for _, event := range []CrawlerWebhookEvent{WebhookCrawlerSearchReady, WebhookCrawlerSearchFailed} {
		if !event.IsValid() {
			t.Errorf("%q rejected by IsValid", event)
		}
		detected, err := DetectCrawlerWebhookEvent([]byte(`{"event": "` + string(event) + `", "payload": {}}`))
		if err != nil {
			t.Errorf("DetectCrawlerWebhookEvent(%q): %v", event, err)
		}
		if detected != event {
			t.Errorf("detected %q, want %q", detected, event)
		}
	}
}

func TestCrawlerUpdatedWebhook_Decodes(t *testing.T) {
	body := []byte(`{
		"event": "crawler_updated",
		"payload": {
			"crawler_uuid": "b4867c50-318c-47cd-bfc9-bed67f24771a",
			"project": "default",
			"env": "LIVE",
			"seed_url": "https://web-scraping.dev/products",
			"action": "updated",
			"state": {"urls_visited": 5},
			"refresh": {
				"at": "2026-09-03T04:12:46.912430Z",
				"generation": 12,
				"added": 1,
				"updated": 2,
				"removed": 1,
				"unchanged": 2,
				"failed": 0,
				"duration_ms": 41870,
				"search_status": "READY"
			},
			"documents": {
				"updated": ["https://web-scraping.dev/product/25", "https://web-scraping.dev/products"],
				"removed": ["https://web-scraping.dev/product/9"],
				"truncated": true
			},
			"links": {"status": "https://api.scrapfly.io/crawl/b4867c50-318c-47cd-bfc9-bed67f24771a/status"}
		}
	}`)

	var wh CrawlerUpdatedWebhook
	if err := json.Unmarshal(body, &wh); err != nil {
		t.Fatal(err)
	}
	if wh.Event != WebhookCrawlerUpdated {
		t.Errorf("event %q, want %q", wh.Event, WebhookCrawlerUpdated)
	}
	if wh.Payload.CrawlerUUID != "b4867c50-318c-47cd-bfc9-bed67f24771a" {
		t.Errorf("crawler_uuid %q", wh.Payload.CrawlerUUID)
	}
	if wh.Payload.Refresh.Generation != 12 || wh.Payload.Refresh.Changed() != 4 {
		t.Errorf("refresh generation=%d changed=%d", wh.Payload.Refresh.Generation, wh.Payload.Refresh.Changed())
	}
	if len(wh.Payload.Documents.Updated) != 2 || len(wh.Payload.Documents.Removed) != 1 {
		t.Errorf("documents updated=%d removed=%d", len(wh.Payload.Documents.Updated), len(wh.Payload.Documents.Removed))
	}
	// Truncated is the only signal that the counts outrun the lists.
	if !wh.Payload.Documents.Truncated {
		t.Error("truncated must survive the decode")
	}
	if wh.Payload.Links.Status == "" {
		t.Error("links.status must survive the decode")
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

// ==============================================================================
// Search / prompt
// ==============================================================================

const searchEnvelope = `{
  "query": "TLS fingerprint",
  "mode": "hybrid",
  "limit": 20,
  "completeness": "exact",
  "crawls": [{"crawler_uuid": "0198aaaa", "documents": 412, "vectors": 18432, "index": "IVF_PQ"}],
  "skipped": [{"crawler_uuid": "0198bbbb", "reason": "search_not_ready", "status": "BUILDING"}],
  "results": [{
    "rank": 1,
    "score": 0.927,
    "scores": {"vector": 0.91, "fts": 12.4, "rrf": 0.0312},
    "crawler_uuid": "0198aaaa",
    "url": "https://example.com/foo",
    "title": "Foo Product",
    "source_format": "markdown",
    "content_type": "application/markdown",
    "chunk_id": 3,
    "text": "the matched chunk",
    "warc_offset": 728271,
    "warc_end": 746643,
    "contents_url": "https://api.scrapfly.io/crawl/0198aaaa/contents?url=x"
  }],
  "stats": {"duration_ms": 412, "crawls_searched": 1, "candidates": 150, "gcs_gets": 27},
  "crawls_requested": 2,
  "crawls_searched": 1,
  "crawls_pruned_exact": 0,
  "crawls_skipped_deadline": ["0198cccc"],
  "crawls_failed": [{"crawler_uuid": "0198dddd", "reason": "search_failed", "status": "FAILED"}],
  "theta": 0.42,
  "max_ub_unsearched": 0.31,
  "cursor": null
}`

func TestClient_CrawlsSearch_POSTsCollectionBody(t *testing.T) {
	var got map[string]interface{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/crawl/search" {
			t.Errorf("expected /crawl/search, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "__API_KEY__" {
			t.Error("key missing in query")
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchEnvelope))
	})

	res, err := client.CrawlsSearch([]string{"0198aaaa", "0198bbbb"}, "TLS fingerprint", &CrawlSearchOptions{
		Limit:   20,
		Mode:    CrawlerSearchModeHybrid,
		Filters: map[string]interface{}{"url_prefix": "https://example.com/docs/"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got["query"] != "TLS fingerprint" {
		t.Errorf("query: %v", got["query"])
	}
	ids, _ := got["crawl_ids"].([]interface{})
	if len(ids) != 2 || ids[0] != "0198aaaa" || ids[1] != "0198bbbb" {
		t.Errorf("crawl_ids: %v", got["crawl_ids"])
	}
	if got["limit"] != float64(20) {
		t.Errorf("limit: %v", got["limit"])
	}
	if got["mode"] != "hybrid" {
		t.Errorf("mode: %v", got["mode"])
	}
	filters, _ := got["filters"].(map[string]interface{})
	if filters["url_prefix"] != "https://example.com/docs/" {
		t.Errorf("filters: %v", got["filters"])
	}

	if !res.IsExact() {
		t.Errorf("completeness: %s", res.Completeness)
	}
	if len(res.Results) != 1 || res.Results[0].URL != "https://example.com/foo" {
		t.Fatalf("results: %+v", res.Results)
	}
	if res.Results[0].Scores.RRF != 0.0312 {
		t.Errorf("rrf: %v", res.Results[0].Scores.RRF)
	}
	if res.Results[0].Scores.Vector == nil || *res.Results[0].Scores.Vector != 0.91 {
		t.Errorf("vector: %v", res.Results[0].Scores.Vector)
	}
	if res.Results[0].Scores.FTS == nil || *res.Results[0].Scores.FTS != 12.4 {
		t.Errorf("fts: %v", res.Results[0].Scores.FTS)
	}
	if res.Results[0].WARCOffset == nil || *res.Results[0].WARCOffset != 728271 {
		t.Errorf("warc_offset: %v", res.Results[0].WARCOffset)
	}
	if res.Results[0].WARCEnd == nil || *res.Results[0].WARCEnd != 746643 {
		t.Errorf("warc_end: %v", res.Results[0].WARCEnd)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != CrawlerSkipSearchNotReady {
		t.Errorf("skipped: %+v", res.Skipped)
	}
	if res.Crawls[0].Vectors != 18432 {
		t.Errorf("crawls: %+v", res.Crawls)
	}
	// The deadline and failure envelopes name their crawls, so a caller can
	// retry exactly those. Counting them would say how many without saying
	// which.
	if len(res.CrawlsSkippedDeadline) != 1 || res.CrawlsSkippedDeadline[0] != "0198cccc" {
		t.Errorf("crawls_skipped_deadline: %+v", res.CrawlsSkippedDeadline)
	}
	if len(res.CrawlsFailed) != 1 || res.CrawlsFailed[0].CrawlerUUID != "0198dddd" ||
		res.CrawlsFailed[0].Reason != CrawlerSkipSearchFailed {
		t.Errorf("crawls_failed: %+v", res.CrawlsFailed)
	}
	if res.Theta == nil || *res.Theta != 0.42 {
		t.Errorf("theta: %v", res.Theta)
	}
	if res.MaxUBUnsearched == nil || *res.MaxUBUnsearched != 0.31 {
		t.Errorf("max_ub_unsearched: %v", res.MaxUBUnsearched)
	}
	if res.Cursor != "" {
		t.Errorf("cursor: %q", res.Cursor)
	}
}

func TestParseCrawlerSearch_EmptyFanOutEnvelope(t *testing.T) {
	// A search that opened no crawl still answers 200 with the whole envelope:
	// the two skip lists come back as empty arrays and the two bounds as null,
	// because no crawl was opened to compute one. This is what the API returns
	// for a single crawl whose index is DISABLED, so it is the shape every
	// caller meets first.
	body := []byte(`{
	  "query": "product price", "mode": "hybrid", "limit": 10, "completeness": "exact",
	  "crawls": [], "results": [],
	  "skipped": [{"crawler_uuid": "0198aaaa", "reason": "search_disabled", "status": "DISABLED"}],
	  "stats": {"duration_ms": 7, "crawls_searched": 0, "candidates": 0, "gcs_gets": 0},
	  "crawls_requested": 1, "crawls_searched": 0, "crawls_pruned_exact": 0,
	  "crawls_skipped_deadline": [], "crawls_failed": [],
	  "theta": null, "max_ub_unsearched": null, "cursor": null
	}`)
	res, err := parseCrawlerSearch(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CrawlsSkippedDeadline) != 0 || len(res.CrawlsFailed) != 0 {
		t.Errorf("empty fan-out reported skips: %+v %+v", res.CrawlsSkippedDeadline, res.CrawlsFailed)
	}
	// nil says no bound was computed. 0 would claim the best unsearched score
	// was zero, which is a different and much stronger statement.
	if res.Theta != nil || res.MaxUBUnsearched != nil {
		t.Errorf("bounds invented for an unopened fan-out: theta=%v max_ub=%v", res.Theta, res.MaxUBUnsearched)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != CrawlerSkipSearchDisabled {
		t.Errorf("skipped: %+v", res.Skipped)
	}
}

func TestParseCrawlerSearch_SingleLegHitKeepsNullsDistinct(t *testing.T) {
	// A hybrid search fuses two legs, and a candidate only one leg retrieved
	// carries null for the other. The chunk here also has no WARC byte range,
	// which is what the engine sends for a document it indexed without writing
	// to the artifact. Every one of those nulls has a plausible zero, so a
	// non-pointer model turns "never scored" into "scored 0" and "no range"
	// into a range read at offset 0.
	body := []byte(`{
	  "query": "product price", "mode": "hybrid", "limit": 10, "completeness": "exact",
	  "crawls": [{"crawler_uuid": "0198aaaa", "documents": 4, "vectors": 12, "index": ""}],
	  "skipped": [],
	  "results": [{
	    "rank": 1, "score": 0.03,
	    "scores": {"vector": 0.91, "fts": null, "rrf": 0.03},
	    "crawler_uuid": "0198aaaa", "url": "https://example.com/foo", "title": "Foo",
	    "source_format": "markdown", "content_type": "application/markdown",
	    "chunk_id": 0, "text": "the matched chunk",
	    "warc_offset": null, "warc_end": null,
	    "contents_url": "https://api.scrapfly.io/crawl/0198aaaa/contents?url=x"
	  }],
	  "stats": {"duration_ms": 9, "crawls_searched": 1, "candidates": 1, "gcs_gets": 1},
	  "crawls_requested": 1, "crawls_searched": 1, "crawls_pruned_exact": 0,
	  "crawls_skipped_deadline": [], "crawls_failed": [],
	  "theta": 0.03, "max_ub_unsearched": null, "cursor": null
	}`)
	res, err := parseCrawlerSearch(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("results: %+v", res.Results)
	}
	hit := res.Results[0]
	if hit.Scores.Vector == nil || *hit.Scores.Vector != 0.91 {
		t.Errorf("the leg that did score the chunk lost its score: %v", hit.Scores.Vector)
	}
	if hit.Scores.FTS != nil {
		t.Errorf("the leg that never saw the chunk reports a score of %v", *hit.Scores.FTS)
	}
	if hit.WARCOffset != nil || hit.WARCEnd != nil {
		t.Errorf("a chunk with no byte range decoded one: offset=%v end=%v", hit.WARCOffset, hit.WARCEnd)
	}
}

func TestClient_CrawlSearch_IsAOneElementCollectionCall(t *testing.T) {
	// The collection endpoint is the real API; the single-crawl call must not
	// grow a path of its own or the two can answer differently.
	var got map[string]interface{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl/search" {
			t.Errorf("expected /crawl/search, got %s", r.URL.Path)
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchEnvelope))
	})

	if _, err := client.CrawlSearch("0198aaaa", "TLS fingerprint", nil); err != nil {
		t.Fatal(err)
	}
	ids, _ := got["crawl_ids"].([]interface{})
	if len(ids) != 1 || ids[0] != "0198aaaa" {
		t.Errorf("crawl_ids: %v", got["crawl_ids"])
	}
}

func TestClient_CrawlsSearch_RejectsBadInputBeforeRequest(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be issued")
	})

	if _, err := client.CrawlsSearch(nil, "q", nil); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("empty crawl_ids: %v", err)
	}
	if _, err := client.CrawlsSearch([]string{"a", "a"}, "q", nil); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("duplicate crawl_ids: %v", err)
	}
	if _, err := client.CrawlsSearch([]string{"a"}, "", nil); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("empty query: %v", err)
	}
	if _, err := client.CrawlsSearch([]string{"a"}, "q", &CrawlSearchOptions{Mode: "semantic"}); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("invalid mode: %v", err)
	}
}

func TestClient_CrawlsSearch_ErrorEnvelopeWraps(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code": "ERR::CRAWLER::SEARCH_NOT_ENABLED", "message": "Search is not enabled"}`))
	})
	_, err := client.CrawlsSearch([]string{"0198aaaa"}, "q", nil)
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Fatalf("expected ErrCrawlerFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "SEARCH_NOT_ENABLED") {
		t.Errorf("error lost the code: %v", err)
	}
}

func TestClient_CrawlsPrompt_StreamsFrames(t *testing.T) {
	var got map[string]interface{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl/prompt" {
			t.Errorf("expected /crawl/prompt, got %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept: %s", r.Header.Get("Accept"))
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: source\ndata: {\"id\":1,\"crawler_uuid\":\"0198aaaa\",\"url\":\"https://example.com/foo\",\"score\":0.92}\n\n" +
			":keepalive\n\n" +
			"event: token\ndata: \"The\"\n\n" +
			"event: token\ndata: \" answer\"\n\n" +
			"event: done\ndata: {\"sources_used\":[1],\"sources_dropped\":2,\"truncated\":false," +
			"\"api_credit\":3," +
			"\"usage\":{\"prompt_token_count\":30,\"candidates_token_count\":9,\"thoughts_token_count\":3," +
			"\"total_token_count\":42,\"cost\":{\"input\":0.000012,\"output\":0.000048}," +
			"\"model\":\"gemini-2.5-flash\"}}\n\n"))
	})

	var (
		answer  strings.Builder
		sources []CrawlerPromptSource
		done    CrawlerPromptDone
	)
	err := client.CrawlsPrompt([]string{"0198aaaa", "0198bbbb"}, "Compare the pricing models.",
		&CrawlPromptOptions{
			Search: &CrawlSearchOptions{Limit: 30, Mode: CrawlerSearchModeHybrid},
			Model:  "gemini-2.5-flash-lite",
		},
		func(ev CrawlerPromptEvent) error {
			switch ev.Type {
			case CrawlerPromptEventToken:
				answer.WriteString(ev.Token)
			case CrawlerPromptEventSource:
				sources = append(sources, ev.Source)
			case CrawlerPromptEventDone:
				done = ev.Done
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}

	generation, _ := got["generation"].(map[string]interface{})
	if generation["stream"] != true || generation["model"] != "gemini-2.5-flash-lite" {
		t.Errorf("generation: %v", got["generation"])
	}
	search, _ := got["search"].(map[string]interface{})
	if search["limit"] != float64(30) || search["mode"] != "hybrid" {
		t.Errorf("search: %v", got["search"])
	}

	if answer.String() != "The answer" {
		t.Errorf("answer: %q", answer.String())
	}
	if len(sources) != 1 || sources[0].URL != "https://example.com/foo" {
		t.Errorf("sources: %+v", sources)
	}
	if len(done.SourcesUsed) != 1 || done.SourcesUsed[0] != 1 {
		t.Errorf("sources_used: %+v", done.SourcesUsed)
	}
	// The done frame reports the flat price and nothing about how the answer
	// was produced. The fixture above still sends usage, tokens, cost and the
	// model precisely because an older API will: decoding into a struct that
	// has no such field is what drops them.
	if strings.Contains(fmt.Sprintf("%+v", done), "gemini") {
		t.Errorf("done frame leaks the model: %+v", done)
	}
	// Dropped sources were retrieved and ranked but never shown to the model.
	if done.SourcesDropped != 2 {
		t.Errorf("sources_dropped: %d", done.SourcesDropped)
	}
	// api_credit is the one cost fact the API does publish, and it is what the
	// run was actually charged rather than the list price.
	if done.APICredit == nil || *done.APICredit != 3 {
		t.Errorf("api_credit: %v", done.APICredit)
	}
}

// An engine too old to report the charge sends no api_credit key. That is not
// the same fact as a zero charge, so it must stay nil rather than decode to 0 —
// a caller printing 0 would be reporting a free run that was billed.
func TestClient_CrawlsPrompt_DoneWithoutAPICreditStaysNil(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: done\ndata: {\"sources_used\":[],\"sources_dropped\":0,\"truncated\":false}\n\n"))
	})

	var done CrawlerPromptDone
	err := client.CrawlsPrompt([]string{"0198aaaa"}, "anything", nil, func(ev CrawlerPromptEvent) error {
		if ev.Type == CrawlerPromptEventDone {
			done = ev.Done
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if done.APICredit != nil {
		t.Errorf("api_credit: want nil for an engine that does not report it, got %d", *done.APICredit)
	}
}

func TestClient_CrawlsPrompt_ErrorFrameFailsMidStream(t *testing.T) {
	// Generation can fail after tokens were already handed to the caller.
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: token\ndata: \"partial\"\n\n" +
			"event: error\ndata: {\"code\":\"ERR::CRAWLER::PROMPT_GENERATION_FAILED\",\"message\":\"upstream refused\"}\n\n"))
	})

	var tokens []string
	err := client.CrawlsPrompt([]string{"0198aaaa"}, "hi", nil, func(ev CrawlerPromptEvent) error {
		if ev.Type == CrawlerPromptEventToken {
			tokens = append(tokens, ev.Token)
		}
		return nil
	})
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Fatalf("expected ErrCrawlerFailed, got %v", err)
	}
	if len(tokens) != 1 || tokens[0] != "partial" {
		t.Errorf("tokens delivered before the failure: %+v", tokens)
	}
}

func TestClient_CrawlsPrompt_RequiresDoneFrame(t *testing.T) {
	for _, body := range []string{
		"",
		":keepalive\n\n",
		"event: token\ndata: \"partial\"\n\n",
		"event: token\ndata: \"partial\"\n\nevent: done\ndata: {}\n",
	} {
		t.Run(body, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, body)
			})
			var answer strings.Builder
			err := client.CrawlsPrompt([]string{"abc"}, "question", nil, func(ev CrawlerPromptEvent) error {
				if ev.Type == CrawlerPromptEventToken {
					answer.WriteString(ev.Token)
				}
				return nil
			})
			if !errors.Is(err, ErrCrawlerFailed) || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("expected crawler failure wrapping unexpected EOF, got %v", err)
			}
			if strings.Contains(body, "partial") && answer.String() != "partial" {
				t.Errorf("lost tokens delivered before EOF: %q", answer.String())
			}
		})
	}
}

func TestClient_CrawlsPrompt_HandlerErrorStopsStream(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: token\ndata: \"a\"\n\nevent: token\ndata: \"b\"\n\n"))
	})

	stop := errors.New("enough")
	count := 0
	err := client.CrawlsPrompt([]string{"0198aaaa"}, "hi", nil, func(ev CrawlerPromptEvent) error {
		count++
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("expected the handler's error, got %v", err)
	}
	if count != 1 {
		t.Errorf("handler called %d times after asking to stop", count)
	}
}

func TestCrawl_SearchAndPrompt_DelegateToClient(t *testing.T) {
	paths := map[string]bool{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths[r.URL.Path] = true
		switch r.URL.Path {
		case "/crawl":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"crawler_uuid": "abc", "status": "PENDING"}`))
		case "/crawl/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(searchEnvelope))
		case "/crawl/prompt":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: done\ndata: {\"truncated\":false}\n\n"))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})

	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com", Search: true})
	if err := crawl.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := crawl.Search("q", nil); err != nil {
		t.Errorf("Search: %v", err)
	}
	if err := crawl.Prompt("q", nil, func(CrawlerPromptEvent) error { return nil }); err != nil {
		t.Errorf("Prompt: %v", err)
	}
	if !paths["/crawl/search"] || !paths["/crawl/prompt"] {
		t.Errorf("endpoints not reached: %v", paths)
	}
}

func TestCrawl_SearchAndPrompt_NotStartedReturnError(t *testing.T) {
	client, _ := New("__API_KEY__")
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com", Search: true})

	if _, err := crawl.Search("q", nil); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("Search: %v", err)
	}
	if err := crawl.Prompt("q", nil, func(CrawlerPromptEvent) error { return nil }); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("Prompt: %v", err)
	}
}

func TestParseCrawlerStatus_SearchBlock(t *testing.T) {
	body := []byte(`{
	  "crawler_uuid": "abc", "status": "DONE", "is_finished": true, "is_success": true,
	  "state": {"urls_visited": 1, "urls_extracted": 1, "urls_failed": 0, "urls_skipped": 0,
	            "urls_to_crawl": 0, "api_credit_used": 1, "duration": 1},
	  "search": {"status": "READY", "documents": 412, "vectors": 18432, "index": "IVF_PQ", "generation": 1}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Search.IsSearchable() {
		t.Fatalf("search: %+v", status.Search)
	}
	if status.Search.Vectors != 18432 || status.Search.Generation != 1 {
		t.Errorf("search: %+v", status.Search)
	}
}

func TestParseCrawlerStatus_SearchBlockCarriesTheWriterBacklog(t *testing.T) {
	// queue_depth and fragments ride every status poll. They are what tells a
	// caller watching a BUILDING index that the writer is still draining
	// rather than stalled, so a model that omits them reports both as 0 and
	// the two conditions become indistinguishable.
	body := []byte(`{
	  "crawler_uuid": "abc", "status": "RUNNING", "is_finished": false, "is_success": null,
	  "state": {"urls_visited": 12, "urls_extracted": 40, "urls_failed": 0, "urls_skipped": 0,
	            "urls_to_crawl": 28, "api_credit_used": 12, "duration": 30},
	  "search": {"status": "BUILDING", "documents": 12, "vectors": 240, "dropped": 1,
	             "fragments": 3, "queue_depth": 17, "embedding_model": "gemini-embedding-001",
	             "embedding_dimension": 768,
	             "manifest": "uid/default/test/crawler/abc/search/manifest.json"}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if status.Search.QueueDepth != 17 || status.Search.Fragments != 3 {
		t.Errorf("search backlog: %+v", status.Search)
	}
	// The status block must not surface the embedding model to callers.
	if strings.Contains(fmt.Sprintf("%+v", status.Search), "gemini") {
		t.Errorf("status leaks the embedding model: %+v", status.Search)
	}
	if status.Search.Manifest == "" {
		t.Errorf("manifest: %+v", status.Search)
	}
	if status.Search.IsSearchable() {
		t.Error("a BUILDING index must not read as searchable")
	}
}

func TestParseCrawlerStatus_SearchAbsentOnCrawlWithoutIt(t *testing.T) {
	body := []byte(`{
	  "crawler_uuid": "abc", "status": "DONE", "is_finished": true, "is_success": true,
	  "state": {"urls_visited": 1, "urls_extracted": 1, "urls_failed": 0, "urls_skipped": 0,
	            "urls_to_crawl": 0, "api_credit_used": 1, "duration": 1}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if status.Search != nil {
		t.Errorf("expected nil search block, got %+v", status.Search)
	}
	if status.Search.IsSearchable() {
		t.Error("a nil search block must not read as searchable")
	}
}

func TestClient_PromptHTTPClient_OutlivesTheServerBudget(t *testing.T) {
	// The API allows retrieval plus up to 150s of generation under a 165s
	// ceiling, and http.Client.Timeout covers the body, so the SDK default
	// would cut a legitimate stream mid-answer.
	client, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	if got := client.promptHTTPClient().Timeout; got != crawlPromptStreamTimeout {
		t.Errorf("prompt timeout = %s, want %s", got, crawlPromptStreamTimeout)
	}
	if client.httpClient.Timeout != 150*time.Second {
		t.Errorf("the shared client was mutated: %s", client.httpClient.Timeout)
	}

	// A caller who configured their own longer deadline keeps it.
	transport := &http.Transport{}
	client.SetHTTPClient(&http.Client{Timeout: 10 * time.Minute, Transport: transport})
	streaming := client.promptHTTPClient()
	if streaming.Timeout != 10*time.Minute {
		t.Errorf("caller timeout overridden: %s", streaming.Timeout)
	}

	// No timeout at all means the caller opted out; do not impose one.
	client.SetHTTPClient(&http.Client{Transport: transport})
	if got := client.promptHTTPClient().Timeout; got != 0 {
		t.Errorf("imposed a timeout on an unbounded client: %s", got)
	}

	// The shallow copy keeps the injected Transport, so the connection pool
	// and any logging RoundTripper survive.
	client.SetHTTPClient(&http.Client{Timeout: time.Second, Transport: transport})
	if client.promptHTTPClient().Transport != transport {
		t.Error("streaming copy dropped the injected Transport")
	}
}

// ==============================================================================
// Auto-refresh
// ==============================================================================

const refreshEnvelope = `{
  "enabled": true,
  "interval_seconds": 86400,
  "status": "SCHEDULED",
  "generation": 2,
  "last_run_at": "2026-09-01T04:00:00Z",
  "next_run_at": "2026-09-02T04:00:00Z",
  "history": [
    {"at": "2026-08-31T04:00:00Z", "generation": 1, "added": 0, "updated": 0, "removed": 0,
     "unchanged": 412, "failed": 0, "duration_ms": 41200, "search_status": "READY"},
    {"at": "2026-09-01T04:00:00Z", "generation": 2, "added": 3, "updated": 7, "removed": 1,
     "unchanged": 404, "failed": 0, "duration_ms": 44900, "search_status": "READY",
     "sample_updated": ["https://example.com/pricing"],
     "sample_removed": ["https://example.com/old"]}
  ]
}`

func TestCrawlerConfig_RefreshSerializes(t *testing.T) {
	body, err := (&CrawlerConfig{URL: "https://example.com", Refresh: true, RefreshInterval: 86400}).toJSONBody()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["refresh"] != true {
		t.Errorf("refresh not on the wire: %v", decoded["refresh"])
	}
	if decoded["refresh_interval"] != float64(86400) {
		t.Errorf("refresh_interval not on the wire: %v", decoded["refresh_interval"])
	}
}

func TestCrawlerConfig_RefreshOmittedWhenOff(t *testing.T) {
	// Unset means server default: never emit a field to send its default.
	body, err := (&CrawlerConfig{URL: "https://example.com"}).toJSONBody()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, present := decoded["refresh"]; present {
		t.Error("refresh must be absent when not requested")
	}
	if _, present := decoded["refresh_interval"]; present {
		t.Error("refresh_interval must be absent when not requested")
	}
}

func TestCrawlerConfig_RefreshIntervalBounds(t *testing.T) {
	for _, interval := range []int{1, CrawlerRefreshMinInterval - 1, CrawlerRefreshMaxInterval + 1} {
		config := &CrawlerConfig{URL: "https://example.com", Refresh: true, RefreshInterval: interval}
		if _, err := config.toJSONBody(); !errors.Is(err, ErrCrawlerConfig) {
			t.Errorf("interval %d accepted: %v", interval, err)
		}
	}
	for _, interval := range []int{CrawlerRefreshMinInterval, 86400, CrawlerRefreshMaxInterval} {
		config := &CrawlerConfig{URL: "https://example.com", Refresh: true, RefreshInterval: interval}
		if _, err := config.toJSONBody(); err != nil {
			t.Errorf("interval %d rejected: %v", interval, err)
		}
	}
	// A period with the feature off would silently never run.
	orphan := &CrawlerConfig{URL: "https://example.com", RefreshInterval: 86400}
	if _, err := orphan.toJSONBody(); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("interval without Refresh accepted: %v", err)
	}
}

func TestClient_CrawlRefreshNow_POSTsToTheCrawl(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/crawl/0198aaaa/refresh" {
			t.Errorf("expected /crawl/0198aaaa/refresh, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "__API_KEY__" {
			t.Error("key missing in query")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(refreshEnvelope))
	})

	state, err := client.CrawlRefreshNow("0198aaaa")
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || state.Status != CrawlerRefreshScheduled || state.Generation != 2 {
		t.Fatalf("state: %+v", state)
	}
	if state.NextRunAt != "2026-09-02T04:00:00Z" {
		t.Errorf("next_run_at: %q", state.NextRunAt)
	}
	last := state.LastRun()
	if last == nil || last.Updated != 7 || last.Changed() != 11 {
		t.Fatalf("last run: %+v", last)
	}
	if len(last.SampleRemoved) != 1 || last.SampleRemoved[0] != "https://example.com/old" {
		t.Errorf("sample_removed: %v", last.SampleRemoved)
	}
}

func TestClient_CrawlRefreshSettings_PATCHesOnlyWhatIsSet(t *testing.T) {
	var got map[string]interface{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/crawl/0198aaaa/refresh" {
			t.Errorf("expected /crawl/0198aaaa/refresh, got %s", r.URL.Path)
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(refreshEnvelope))
	})

	interval := 86400
	if _, err := client.CrawlRefreshSettings("0198aaaa", CrawlRefreshSettings{
		Enabled:         BoolPtr(true),
		IntervalSeconds: &interval,
	}); err != nil {
		t.Fatal(err)
	}
	if got["refresh"] != true || got["refresh_interval"] != float64(86400) {
		t.Errorf("body: %v", got)
	}

	// Turning refresh off must not send an interval, or it would overwrite the
	// period the crawl keeps for when it is turned back on.
	got = nil
	if _, err := client.CrawlRefreshSettings("0198aaaa", CrawlRefreshSettings{
		Enabled: BoolPtr(false),
	}); err != nil {
		t.Fatal(err)
	}
	if got["refresh"] != false {
		t.Errorf("body: %v", got)
	}
	if _, present := got["refresh_interval"]; present {
		t.Errorf("interval leaked into an enable-only patch: %v", got)
	}
}

func TestClient_CrawlRefresh_RejectsBadInputBeforeRequest(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be issued")
	})

	if _, err := client.CrawlRefreshNow(""); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("empty uuid: %v", err)
	}
	if _, err := client.CrawlRefreshSettings("a", CrawlRefreshSettings{}); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("empty patch: %v", err)
	}
	tooShort := 60
	if _, err := client.CrawlRefreshSettings("a", CrawlRefreshSettings{IntervalSeconds: &tooShort}); !errors.Is(err, ErrCrawlerConfig) {
		t.Errorf("out-of-bounds interval: %v", err)
	}
}

func TestClient_CrawlRefreshHistory_ReturnsTheTimelineNewestLast(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/crawl/0198aaaa/refresh/history" {
			t.Errorf("expected /crawl/0198aaaa/refresh/history, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit: %q", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(refreshEnvelope))
	})

	history, err := client.CrawlRefreshHistory("0198aaaa", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Generation != 1 || history[1].Generation != 2 {
		t.Fatalf("history: %+v", history)
	}
	if history[0].Changed() != 0 || history[0].Unchanged != 412 {
		t.Errorf("a run where nothing changed: %+v", history[0])
	}
}

func TestClient_CrawlRefresh_ErrorEnvelopeWraps(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code": "ERR::CRAWLER::REFRESH_IN_PROGRESS", "message": "A refresh is already running"}`))
	})
	_, err := client.CrawlRefreshNow("0198aaaa")
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Fatalf("expected ErrCrawlerFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), "REFRESH_IN_PROGRESS") {
		t.Errorf("error lost the code: %v", err)
	}
}

func TestCrawl_Refresh_DelegatesToClient(t *testing.T) {
	paths := map[string]bool{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/crawl":
			_, _ = w.Write([]byte(`{"crawler_uuid": "abc", "status": "PENDING"}`))
		case "/crawl/abc/refresh", "/crawl/abc/refresh/history":
			_, _ = w.Write([]byte(refreshEnvelope))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})

	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com", Refresh: true})
	if err := crawl.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := crawl.RefreshNow(); err != nil {
		t.Errorf("RefreshNow: %v", err)
	}
	if _, err := crawl.RefreshSettings(CrawlRefreshSettings{Enabled: BoolPtr(true)}); err != nil {
		t.Errorf("RefreshSettings: %v", err)
	}
	if _, err := crawl.RefreshHistory(0); err != nil {
		t.Errorf("RefreshHistory: %v", err)
	}
	if !paths["/crawl/abc/refresh"] || !paths["/crawl/abc/refresh/history"] {
		t.Errorf("endpoints not reached: %v", paths)
	}
}

func TestCrawl_Refresh_NotStartedReturnsError(t *testing.T) {
	client, _ := New("__API_KEY__")
	crawl := NewCrawl(client, &CrawlerConfig{URL: "https://example.com", Refresh: true})

	if _, err := crawl.RefreshNow(); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("RefreshNow: %v", err)
	}
	if _, err := crawl.RefreshSettings(CrawlRefreshSettings{Enabled: BoolPtr(true)}); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("RefreshSettings: %v", err)
	}
	if _, err := crawl.RefreshHistory(0); !errors.Is(err, ErrCrawlerNotStarted) {
		t.Errorf("RefreshHistory: %v", err)
	}
}

func TestParseCrawlerStatus_RefreshBlock(t *testing.T) {
	body := []byte(`{
	  "crawler_uuid": "abc", "status": "DONE", "is_finished": true, "is_success": true,
	  "state": {"urls_visited": 1, "urls_extracted": 1, "urls_failed": 0, "urls_skipped": 0,
	            "urls_to_crawl": 0, "api_credit_used": 1, "duration": 1},
	  "refresh": {"enabled": true, "interval_seconds": 86400, "status": "SCHEDULED", "generation": 2,
	              "next_run_at": "2026-09-02T04:00:00Z"}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if status.Refresh == nil || !status.Refresh.Enabled || status.Refresh.IntervalSeconds != 86400 {
		t.Fatalf("refresh: %+v", status.Refresh)
	}
	if status.Refresh.IsRunning() {
		t.Error("a SCHEDULED crawl must not read as running")
	}
}

func TestParseCrawlerStatus_RefreshBlockCarriesTheRunFields(t *testing.T) {
	// started_at and consecutive_failures are route-scoped: /status relays the
	// engine's refresh block verbatim and carries both, while the three
	// refresh calls render the API's own typed state, which declares neither.
	// One SDK type serves both routes, so both fields have to survive the
	// route that sends them and stay zero on the route that does not.
	body := []byte(`{
	  "crawler_uuid": "abc", "status": "DONE", "is_finished": true, "is_success": true,
	  "state": {"urls_visited": 1, "urls_extracted": 1, "urls_failed": 0, "urls_skipped": 0,
	            "urls_to_crawl": 0, "api_credit_used": 1, "duration": 1},
	  "refresh": {"enabled": true, "interval_seconds": 3600, "status": "RUNNING", "generation": 4,
	              "started_at": "2026-09-03T22:31:03.851147Z", "consecutive_failures": 2,
	              "last_run_at": "2026-09-03T21:31:03Z", "next_run_at": null,
	              "error": "upstream timeout", "history": []}
	}`)
	status, err := parseCrawlerStatus(body)
	if err != nil {
		t.Fatal(err)
	}
	if status.Refresh.StartedAt != "2026-09-03T22:31:03.851147Z" {
		t.Errorf("started_at: %+v", status.Refresh)
	}
	// A run in flight is the only state that sets started_at, which is what
	// makes "how long has this been running" answerable at all.
	if !status.Refresh.IsRunning() {
		t.Errorf("status: %+v", status.Refresh)
	}
	// Two failures in a row separate one flaky run from a dead schedule.
	if status.Refresh.ConsecutiveFailures != 2 {
		t.Errorf("consecutive_failures: %+v", status.Refresh)
	}

	// The refresh calls answer without either key. Absent must decode as the
	// zero value rather than being invented, or a caller reads a healthy
	// schedule off a response that never made the claim.
	callAnswer, err := parseCrawlerRefreshState([]byte(refreshEnvelope))
	if err != nil {
		t.Fatal(err)
	}
	if callAnswer.StartedAt != "" || callAnswer.ConsecutiveFailures != 0 {
		t.Errorf("refresh-call answer invented run fields: %+v", callAnswer)
	}
}

func TestParseCrawlerRefreshState_AcceptsBothEnvelopeShapes(t *testing.T) {
	// The refresh endpoints answer with the state at the top level; /status
	// nests it under "refresh". Both must decode to the same thing.
	flat, err := parseCrawlerRefreshState([]byte(refreshEnvelope))
	if err != nil {
		t.Fatal(err)
	}
	nested, err := parseCrawlerRefreshState([]byte(`{"crawler_uuid": "abc", "refresh": ` + refreshEnvelope + `}`))
	if err != nil {
		t.Fatal(err)
	}
	if flat.Generation != nested.Generation || len(flat.History) != len(nested.History) {
		t.Errorf("shapes disagree: %+v vs %+v", flat, nested)
	}
}
