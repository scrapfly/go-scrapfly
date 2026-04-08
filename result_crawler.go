package scrapfly

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ==============================================================================
// Status
// ==============================================================================

// Crawler status values returned by GET /crawl/{uuid}/status.
//
// Note: there is no "COMPLETED" or "FAILED" status — a finished crawl is
// always status=DONE, and success vs. failure is signaled by is_success.
// CANCELLED is a separate terminal state set when the user cancels the job.
const (
	CrawlerStatusPending   = "PENDING"
	CrawlerStatusRunning   = "RUNNING"
	CrawlerStatusDone      = "DONE"
	CrawlerStatusCancelled = "CANCELLED"
)

// Documented values for CrawlerState.StopReason.
const (
	CrawlerStopNoMoreURLs      = "no_more_urls"
	CrawlerStopPageLimit       = "page_limit"
	CrawlerStopMaxDuration     = "max_duration"
	CrawlerStopMaxAPICredit    = "max_api_credit"
	CrawlerStopSeedURLFailed   = "seed_url_failed"
	CrawlerStopUserCancelled   = "user_cancelled"
	CrawlerStopCrawlerError    = "crawler_error"
	CrawlerStopNoAPICreditLeft = "no_api_credit_left"
	CrawlerStopStorageError    = "storage_error"
)

// CrawlerStartResponse is the response from POST /crawl.
type CrawlerStartResponse struct {
	CrawlerUUID string `json:"crawler_uuid"`
	Status      string `json:"status"`
}

// CrawlerState holds the per-job metrics returned inside CrawlerStatus.
//
// Note on nullable fields: while the crawler is in PENDING (before any worker
// has picked up the job), StartTime, StopTime, and StopReason are all null
// on the wire and zero here (with HasStarted()/HasStopped() for disambiguation).
// They become populated once the crawl progresses.
type CrawlerState struct {
	URLsVisited   int `json:"urls_visited"`
	URLsExtracted int `json:"urls_extracted"`
	URLsFailed    int `json:"urls_failed"`
	URLsSkipped   int `json:"urls_skipped"`
	URLsToCrawl   int `json:"urls_to_crawl"`
	APICreditUsed int `json:"api_credit_used"`
	Duration      int `json:"duration"`

	// StartTime is the Unix timestamp when the first worker picked up the job.
	// nil while the crawler is still PENDING.
	StartTime *int64 `json:"start_time"`

	// StopTime is the Unix timestamp when the crawler reached a terminal state.
	// nil until the crawler is finished or cancelled.
	StopTime *int64 `json:"stop_time"`

	// StopReason is one of the documented stop-reason strings. nil while still running.
	StopReason *string `json:"stop_reason"`
}

// HasStarted reports whether the crawler has been picked up by a worker
// (i.e. StartTime is set).
func (s *CrawlerState) HasStarted() bool { return s.StartTime != nil }

// HasStopped reports whether the crawler reached a terminal state
// (i.e. StopTime is set).
func (s *CrawlerState) HasStopped() bool { return s.StopTime != nil }

// CrawlerStatus wraps the JSON response of GET /crawl/{uuid}/status.
//
// Strict parsing: required fields (CrawlerUUID, Status, IsFinished, State
// and its documented counters) are validated after JSON unmarshal. A missing
// or zero required field throws an error so API contract drift surfaces loud
// rather than silently producing a zero-valued Status object.
type CrawlerStatus struct {
	CrawlerUUID string       `json:"crawler_uuid"`
	Status      string       `json:"status"`
	IsFinished  bool         `json:"is_finished"`
	// IsSuccess is nil while the crawler is still running, then bool once terminal.
	// The server occasionally sends `false` during PENDING, so callers should use
	// IsComplete() / IsFailed() rather than checking the field directly.
	IsSuccess *bool         `json:"is_success"`
	State     CrawlerState `json:"state"`
}

// IsRunning reports whether the crawler is still PENDING or RUNNING.
func (s *CrawlerStatus) IsRunning() bool {
	return s.Status == CrawlerStatusPending || s.Status == CrawlerStatusRunning
}

// IsComplete reports whether the crawler finished successfully.
func (s *CrawlerStatus) IsComplete() bool {
	return s.Status == CrawlerStatusDone && s.IsSuccess != nil && *s.IsSuccess
}

// IsFailed reports whether the crawler reached DONE but failed.
func (s *CrawlerStatus) IsFailed() bool {
	return s.Status == CrawlerStatusDone && s.IsSuccess != nil && !*s.IsSuccess
}

// IsCancelled reports whether the crawler was cancelled by the user.
func (s *CrawlerStatus) IsCancelled() bool { return s.Status == CrawlerStatusCancelled }

// ProgressPct returns a rough progress estimate based on visited vs extracted
// URLs (0-100). Returns 0 when nothing has been extracted yet.
func (s *CrawlerStatus) ProgressPct() float64 {
	if s.State.URLsExtracted == 0 {
		return 0
	}
	return float64(s.State.URLsVisited) / float64(s.State.URLsExtracted) * 100
}

// parseCrawlerStatus unmarshals a JSON body into CrawlerStatus and validates
// the required fields per the documented contract.
//
// Go's json.Unmarshal is lenient by default (missing fields become zero values),
// so we explicitly check the minimum fields the docs promise. This catches API
// contract drift at parse time instead of at the point of first use.
func parseCrawlerStatus(body []byte) (*CrawlerStatus, error) {
	var s CrawlerStatus
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("failed to decode crawler status JSON: %w", err)
	}
	// Re-decode into a map to detect missing required keys (because Go's
	// struct decoder can't distinguish "field absent" from "field zero").
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode crawler status JSON into map: %w", err)
	}
	requiredTop := []string{"crawler_uuid", "status", "is_finished", "state"}
	for _, key := range requiredTop {
		if _, ok := raw[key]; !ok {
			return nil, fmt.Errorf("crawler status response missing required field %q", key)
		}
	}
	if s.CrawlerUUID == "" {
		return nil, fmt.Errorf("crawler status response has empty crawler_uuid")
	}
	if s.Status == "" {
		return nil, fmt.Errorf("crawler status response has empty status")
	}

	// State sub-object: require the counters the docs promise.
	var stateRaw map[string]json.RawMessage
	if err := json.Unmarshal(raw["state"], &stateRaw); err != nil {
		return nil, fmt.Errorf("failed to decode crawler status state: %w", err)
	}
	requiredState := []string{
		"urls_visited", "urls_extracted", "urls_failed", "urls_skipped",
		"urls_to_crawl", "api_credit_used", "duration",
	}
	for _, key := range requiredState {
		if _, ok := stateRaw[key]; !ok {
			return nil, fmt.Errorf("crawler status state missing required field %q", key)
		}
	}
	return &s, nil
}

// ==============================================================================
// URLs (GET /crawl/{uuid}/urls) — streaming text response
// ==============================================================================

// CrawlerURLEntry is a single URL record from GET /crawl/{uuid}/urls.
//
// The endpoint streams one record per line as `text/plain`. For `visited`/
// `pending` URLs each line is just the URL; for `failed`/`skipped` URLs the
// line is `url,reason`. Streaming text is intentional — the endpoint is
// expected to scale to millions of records per job where JSON would be too
// expensive both on the server and the client.
type CrawlerURLEntry struct {
	URL    string
	Status string // Echoed from the request filter (visited/pending/failed/skipped)
	Reason string // Only set for failed/skipped URLs; empty otherwise
}

// CrawlerURLs wraps the streaming-text response of GET /crawl/{uuid}/urls.
//
// Page and PerPage are echoes of the caller's request parameters — the wire
// protocol carries no global total count. Use len(URLs) for the page size
// and request further pages by incrementing Page until an empty response.
type CrawlerURLs struct {
	URLs    []CrawlerURLEntry
	Page    int
	PerPage int
}

// parseCrawlerURLs parses a `text/plain` response body into a CrawlerURLs.
//
// - Empty lines are ignored.
// - For visited/pending status each line is one URL.
// - For failed/skipped status each line is `url,reason` (split on the first comma).
// - statusHint is the status filter the caller passed — used to tag each record.
func parseCrawlerURLs(body string, statusHint string, page, perPage int) *CrawlerURLs {
	entries := make([]CrawlerURLEntry, 0)
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		// Some servers terminate lines with \r\n which TrimSpace handles,
		// but an empty line shouldn't produce a record.
		if line == "" {
			continue
		}
		if statusHint == "visited" || statusHint == "pending" {
			entries = append(entries, CrawlerURLEntry{URL: line, Status: statusHint})
			continue
		}
		// failed/skipped → `url,reason` (split on first comma)
		if idx := strings.Index(line, ","); idx != -1 {
			entries = append(entries, CrawlerURLEntry{
				URL:    line[:idx],
				Status: statusHint,
				Reason: line[idx+1:],
			})
		} else {
			entries = append(entries, CrawlerURLEntry{URL: line, Status: statusHint})
		}
	}
	return &CrawlerURLs{URLs: entries, Page: page, PerPage: perPage}
}

// ==============================================================================
// Contents (GET /crawl/{uuid}/contents)
// ==============================================================================

// CrawlerContents wraps the JSON response of GET /crawl/{uuid}/contents
// (bulk mode, plain=false). The Contents map is URL → format → content.
//
// For the single-URL `plain=true` mode, the client returns a raw string
// directly instead of this struct.
type CrawlerContents struct {
	Contents map[string]map[string]string `json:"contents"`
	Links    CrawlerContentsLinks         `json:"links"`
}

// CrawlerContentsLinks is the pagination links block returned with bulk contents.
type CrawlerContentsLinks struct {
	CrawledURLs string `json:"crawled_urls,omitempty"`
	Next        string `json:"next,omitempty"`
	Prev        string `json:"prev,omitempty"`
}

// parseCrawlerContents decodes and strictly validates the JSON contents body.
func parseCrawlerContents(body []byte) (*CrawlerContents, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode crawler contents JSON: %w", err)
	}
	for _, key := range []string{"contents", "links"} {
		if _, ok := raw[key]; !ok {
			return nil, fmt.Errorf("crawler contents response missing required field %q", key)
		}
	}
	var c CrawlerContents
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("failed to decode crawler contents JSON into struct: %w", err)
	}
	return &c, nil
}

// ==============================================================================
// CrawlContent — high-level wrapper for a single crawled URL's content
// ==============================================================================

// CrawlContent is the response object for a single crawled URL fetched via
// `Crawl.Read(url, format)` or the lower-level `Client.CrawlContentsPlain`.
//
// Mirrors the Python SDK's `CrawlContent` class. Provides typed accessors
// for the content, status code, headers, and metadata of a single page.
type CrawlContent struct {
	// URL is the crawled URL.
	URL string

	// Content is the page content in the requested format.
	Content string

	// StatusCode is the HTTP response status code from the original scrape.
	// Zero when the SDK couldn't determine the status (e.g. plain mode
	// fetches don't carry the status, only the content).
	StatusCode int

	// Headers contains the HTTP response headers from the original scrape.
	// Empty in plain-mode fetches; populated when the content comes from a
	// richer source (e.g. WARC artifact, JSON contents envelope).
	Headers map[string]string

	// Duration is the original scrape duration in seconds, or zero if unknown.
	Duration float64

	// LogID is the Scrapfly scrape log ID for debugging, or empty if unknown.
	LogID string

	// Country is the country the request was made from, or empty if unknown.
	Country string

	// CrawlUUID is the parent crawler job UUID.
	CrawlUUID string
}

// LogURL returns the dashboard URL for this scrape's log, or empty if LogID
// is unset.
func (c *CrawlContent) LogURL() string {
	if c.LogID == "" {
		return ""
	}
	return "https://scrapfly.io/dashboard/monitoring/log/" + c.LogID
}

// Success reports whether the original scrape was a 2xx response.
func (c *CrawlContent) Success() bool {
	return c.StatusCode >= 200 && c.StatusCode < 300
}

// Error reports whether the original scrape was a 4xx/5xx response.
func (c *CrawlContent) Error() bool { return c.StatusCode >= 400 }

// Len returns the length of the content string.
func (c *CrawlContent) Len() int { return len(c.Content) }

// String returns the raw content. Lets fmt.Println pretty-print it.
func (c *CrawlContent) String() string { return c.Content }

// ==============================================================================
// Artifact (GET /crawl/{uuid}/artifact)
// ==============================================================================

// CrawlerArtifactType identifies the wire format of a crawler artifact.
type CrawlerArtifactType string

// Artifact type values accepted by GET /crawl/{uuid}/artifact?type=...
const (
	ArtifactTypeWARC CrawlerArtifactType = "warc"
	ArtifactTypeHAR  CrawlerArtifactType = "har"
)

// CrawlerArtifact holds the raw bytes of a downloaded WARC or HAR artifact.
//
// The SDK does NOT bundle WARC/HAR parsers — use a dedicated Go library
// (e.g. nlnwa/gowarc) if you need to walk the records. The Save() method is
// provided for the common case of writing the artifact to disk.
type CrawlerArtifact struct {
	Type CrawlerArtifactType
	Data []byte
}

// Save writes the artifact bytes to a file at the given path.
func (a *CrawlerArtifact) Save(path string) error {
	return os.WriteFile(path, a.Data, 0o644)
}

// Len returns the size of the artifact data in bytes.
func (a *CrawlerArtifact) Len() int { return len(a.Data) }

// writeTo writes the artifact bytes to the provided writer (useful for piping
// directly into an uploader or parser without hitting disk).
func (a *CrawlerArtifact) writeTo(w io.Writer) (int, error) { return w.Write(a.Data) }

// ==============================================================================
// Webhook payloads
// ==============================================================================

// CrawlerWebhookCommon holds the fields shared by every crawler webhook event.
type CrawlerWebhookCommon struct {
	CrawlerUUID string       `json:"crawler_uuid"`
	Project     string       `json:"project"`
	Env         string       `json:"env"`
	Action      string       `json:"action"`
	State       CrawlerState `json:"state"`
}

// CrawlerLifecycleWebhook covers the four "lifecycle" events that share an
// identical payload shape: crawler_started, crawler_stopped, crawler_cancelled,
// crawler_finished. Verified against the example JSONs in the docs.
type CrawlerLifecycleWebhook struct {
	Event   CrawlerWebhookEvent `json:"event"`
	Payload struct {
		CrawlerWebhookCommon
		SeedURL string `json:"seed_url"`
		Links   struct {
			Status string `json:"status"`
		} `json:"links"`
	} `json:"payload"`
}

// CrawlerURLVisitedWebhook is the payload for the crawler_url_visited event.
type CrawlerURLVisitedWebhook struct {
	Event   CrawlerWebhookEvent `json:"event"`
	Payload struct {
		CrawlerWebhookCommon
		URL    string `json:"url"`
		Scrape struct {
			StatusCode int               `json:"status_code"`
			Country    string            `json:"country,omitempty"`
			LogUUID    string            `json:"log_uuid,omitempty"`
			LogURL     string            `json:"log_url,omitempty"`
			Content    map[string]string `json:"content"`
		} `json:"scrape"`
	} `json:"payload"`
}

// CrawlerURLSkippedWebhook is the payload for the crawler_url_skipped event.
type CrawlerURLSkippedWebhook struct {
	Event   CrawlerWebhookEvent `json:"event"`
	Payload struct {
		CrawlerWebhookCommon
		// URLs maps skipped URLs to their skip reason.
		URLs map[string]string `json:"urls"`
	} `json:"payload"`
}

// CrawlerURLDiscoveredWebhook is the payload for the crawler_url_discovered event.
type CrawlerURLDiscoveredWebhook struct {
	Event   CrawlerWebhookEvent `json:"event"`
	Payload struct {
		CrawlerWebhookCommon
		Origin         string   `json:"origin"`
		DiscoveredURLs []string `json:"discovered_urls"`
	} `json:"payload"`
}

// CrawlerURLFailedWebhook is the payload for the crawler_url_failed event.
type CrawlerURLFailedWebhook struct {
	Event   CrawlerWebhookEvent `json:"event"`
	Payload struct {
		CrawlerWebhookCommon
		URL          string                 `json:"url"`
		Error        string                 `json:"error"`
		ScrapeConfig map[string]interface{} `json:"scrape_config"`
		Links        struct {
			Log *string `json:"log"`
		} `json:"links"`
	} `json:"payload"`
}

// DetectCrawlerWebhookEvent peeks into a webhook request body and returns the
// event name without parsing the rest of the payload. Callers can then
// unmarshal into the appropriate typed webhook struct.
//
// Example:
//
//	event, err := scrapfly.DetectCrawlerWebhookEvent(body)
//	if err != nil { return err }
//	switch event {
//	case scrapfly.WebhookCrawlerFinished:
//	    var wh scrapfly.CrawlerLifecycleWebhook
//	    json.Unmarshal(body, &wh)
//	    // ...
//	case scrapfly.WebhookCrawlerURLVisited:
//	    var wh scrapfly.CrawlerURLVisitedWebhook
//	    json.Unmarshal(body, &wh)
//	    // ...
//	}
func DetectCrawlerWebhookEvent(body []byte) (CrawlerWebhookEvent, error) {
	var envelope struct {
		Event CrawlerWebhookEvent `json:"event"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", fmt.Errorf("failed to decode webhook envelope: %w", err)
	}
	if envelope.Event == "" {
		return "", fmt.Errorf("crawler webhook body missing required 'event' field")
	}
	if !envelope.Event.IsValid() {
		return envelope.Event, fmt.Errorf("unknown crawler webhook event: %q", envelope.Event)
	}
	return envelope.Event, nil
}
