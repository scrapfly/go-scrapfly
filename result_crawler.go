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
	CrawlerUUID string `json:"crawler_uuid"`
	Status      string `json:"status"`
	IsFinished  bool   `json:"is_finished"`
	// IsSuccess is nil while the crawler is still running, then bool once terminal.
	// The server occasionally sends `false` during PENDING, so callers should use
	// IsComplete() / IsFailed() rather than checking the field directly.
	IsSuccess *bool        `json:"is_success"`
	State     CrawlerState `json:"state"`
	// Search is nil unless the crawl was started with Search=true. Polling it
	// is the webhook-free way to learn when the index became queryable.
	Search *CrawlerSearchState `json:"search"`
	// Refresh is nil unless the crawl re-scrapes itself on a period.
	Refresh *CrawlerRefreshState `json:"refresh"`
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
// Search (POST /crawl/search) and prompt (POST /crawl/prompt)
// ==============================================================================

// CrawlerSearchMode selects which retrieval legs run.
type CrawlerSearchMode string

// Search modes. Hybrid runs both legs and merges them with reciprocal rank
// fusion; it is the server default when the field is omitted.
const (
	CrawlerSearchModeVector CrawlerSearchMode = "vector"
	CrawlerSearchModeFTS    CrawlerSearchMode = "fts"
	CrawlerSearchModeHybrid CrawlerSearchMode = "hybrid"
)

// IsValid returns true when the mode is one of the documented values.
func (m CrawlerSearchMode) IsValid() bool {
	switch m {
	case CrawlerSearchModeVector, CrawlerSearchModeFTS, CrawlerSearchModeHybrid:
		return true
	}
	return false
}

// String returns the wire-format value.
func (m CrawlerSearchMode) String() string { return string(m) }

// Search index states carried by CrawlerSearchState.Status. Only READY and
// PARTIAL can answer a query.
const (
	CrawlerSearchDisabled = "DISABLED"
	CrawlerSearchBuilding = "BUILDING"
	CrawlerSearchReady    = "READY"
	CrawlerSearchPartial  = "PARTIAL"
	CrawlerSearchFailed   = "FAILED"
)

// CrawlerSearchState describes a crawl's search index. It appears on
// GET /crawl/{uuid}/status and on the two search webhooks.
type CrawlerSearchState struct {
	Status   string `json:"status"`
	Manifest string `json:"manifest,omitempty"`
	// Documents counts crawled pages represented in the index; Vectors counts
	// the embedded chunks they were split into.
	Documents int `json:"documents"`
	Vectors   int `json:"vectors"`
	Dropped   int `json:"dropped"`
	// Fragments counts the index's on-disk data files. QueueDepth counts rows
	// the writer accepted but has not flushed into one yet, so a nonzero
	// QueueDepth on a terminal crawl means the index is still catching up.
	Fragments  int    `json:"fragments"`
	QueueDepth int    `json:"queue_depth"`
	Error      string `json:"error,omitempty"`
	BuiltAt    string `json:"built_at,omitempty"`
	// Index is the vector index type (e.g. IVF_PQ), empty when the row count
	// stayed below the index threshold.
	Index string `json:"index,omitempty"`
	// Generation is bumped when a paused crawl resumes and rebuilds. Results
	// from different generations are not comparable.
	Generation int `json:"generation,omitempty"`
}

// IsSearchable reports whether the index can answer a query right now.
func (s *CrawlerSearchState) IsSearchable() bool {
	return s != nil && (s.Status == CrawlerSearchReady || s.Status == CrawlerSearchPartial)
}

// CrawlerSearchScores holds the per-leg scores behind a result. Which fields
// are populated depends on the mode.
//
// Vector and FTS are pointers because a candidate found by only one leg has no
// score from the other and the engine sends null there. Decoded into a bare
// float64 that null becomes 0.0, which reads as "we scored it and it was
// orthogonal" rather than "that leg never saw it". RRF is always present.
type CrawlerSearchScores struct {
	Vector *float64 `json:"vector"`
	FTS    *float64 `json:"fts"`
	RRF    float64  `json:"rrf"`
}

// CrawlerSearchResult is one matched chunk.
//
// A result is a chunk, not a page: ChunkID orders chunks within one crawled
// document and Text is only the matched slice. Expand a hit back to the whole
// document through ContentsURL, or through WARCOffset/WARCEnd against the
// crawl's WARC artifact.
//
// WARCOffset and WARCEnd are pointers because the index holds no byte range for
// a chunk the crawl never wrote to WARC, and the engine sends null there. Zero
// is a legal offset, so only nil can say the range is absent instead of
// pointing a range read at the head of the artifact.
type CrawlerSearchResult struct {
	Rank         int                 `json:"rank"`
	Score        float64             `json:"score"`
	Scores       CrawlerSearchScores `json:"scores"`
	CrawlerUUID  string              `json:"crawler_uuid"`
	URL          string              `json:"url"`
	Title        string              `json:"title"`
	SourceFormat string              `json:"source_format"`
	ContentType  string              `json:"content_type"`
	ChunkID      int                 `json:"chunk_id"`
	Text         string              `json:"text"`
	WARCOffset   *int64              `json:"warc_offset"`
	WARCEnd      *int64              `json:"warc_end"`
	ContentsURL  string              `json:"contents_url"`
}

// CrawlerSearchCrawl is a crawl that was opened and searched.
type CrawlerSearchCrawl struct {
	CrawlerUUID string `json:"crawler_uuid"`
	Documents   int    `json:"documents"`
	Vectors     int    `json:"vectors"`
	Index       string `json:"index"`
}

// Skip reasons reported per crawl in CrawlerSearchResponse.Skipped.
const (
	CrawlerSkipSearchNotEnabled  = "search_not_enabled"
	CrawlerSkipSearchNotReady    = "search_not_ready"
	CrawlerSkipSearchFailed      = "search_failed"
	CrawlerSkipSearchDisabled    = "search_disabled"
	CrawlerSkipIncompatibleIndex = "incompatible_index"
	CrawlerSkipDeadline          = "deadline"
)

// CrawlerSearchSkipped is a requested crawl that contributed nothing, and why.
//
// A skip is never fatal: the search still answers from the remaining crawls.
type CrawlerSearchSkipped struct {
	CrawlerUUID string `json:"crawler_uuid"`
	Reason      string `json:"reason"`
	Status      string `json:"status"`
}

// CrawlerSearchStats holds the fan-out's timing and IO counters.
type CrawlerSearchStats struct {
	DurationMS     int `json:"duration_ms"`
	CrawlsSearched int `json:"crawls_searched"`
	Candidates     int `json:"candidates"`
	GCSGets        int `json:"gcs_gets"`
}

// CrawlerSearchResponse wraps the JSON response of POST /crawl/search.
//
// The envelope states its own completeness. Completeness "exact" with most
// crawls unopened is the normal outcome: the fan-out proves via an admissible
// bound that the unopened crawls held nothing better. "partial" means the
// deadline cut the fan-out short.
type CrawlerSearchResponse struct {
	Query        string                 `json:"query"`
	Mode         CrawlerSearchMode      `json:"mode"`
	Limit        int                    `json:"limit"`
	Completeness string                 `json:"completeness"`
	Crawls       []CrawlerSearchCrawl   `json:"crawls"`
	Skipped      []CrawlerSearchSkipped `json:"skipped"`
	Results      []CrawlerSearchResult  `json:"results"`
	Stats        CrawlerSearchStats     `json:"stats"`

	CrawlsRequested   int `json:"crawls_requested"`
	CrawlsSearched    int `json:"crawls_searched"`
	CrawlsPrunedExact int `json:"crawls_pruned_exact"`
	// CrawlsSkippedDeadline and CrawlsFailed name the crawls, they do not count
	// them: a caller told "3 failed" has nothing to retry. Both are lists on
	// the wire even when empty, so an int here is a hard unmarshal error that
	// turns every real search response into a parse failure.
	CrawlsSkippedDeadline []string               `json:"crawls_skipped_deadline"`
	CrawlsFailed          []CrawlerSearchSkipped `json:"crawls_failed"`
	// Theta is the fusion cutoff and MaxUBUnsearched the best score an unopened
	// crawl could still hold. The bound only exists once a crawl has been
	// opened, and nil is not the claim 0 makes.
	Theta           *float64 `json:"theta"`
	MaxUBUnsearched *float64 `json:"max_ub_unsearched"`

	// Cursor pages the next slice. Paging is cursor-based: an offset over a
	// partial fan-out would re-run the legs and shift ranks. Empty on the
	// last page.
	Cursor string `json:"cursor"`
}

// IsExact reports whether the ranking is provably complete for the requested
// crawls.
func (r *CrawlerSearchResponse) IsExact() bool { return r.Completeness == "exact" }

// parseCrawlerSearch unmarshals a JSON body into CrawlerSearchResponse and
// validates the fields the contract always carries, so drift surfaces at parse
// time rather than as an empty result list.
func parseCrawlerSearch(body []byte) (*CrawlerSearchResponse, error) {
	var out CrawlerSearchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to decode crawler search JSON: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode crawler search JSON into map: %w", err)
	}
	for _, key := range []string{"query", "mode", "limit", "completeness", "results"} {
		if _, ok := raw[key]; !ok {
			return nil, fmt.Errorf("crawler search response missing required field %q", key)
		}
	}
	return &out, nil
}

// CrawlerPromptEventType names the SSE frames of POST /crawl/prompt.
type CrawlerPromptEventType string

// Prompt stream frames, in emission order: sources first, then tokens, then
// exactly one done (or one error).
const (
	CrawlerPromptEventSource CrawlerPromptEventType = "source"
	CrawlerPromptEventToken  CrawlerPromptEventType = "token"
	CrawlerPromptEventError  CrawlerPromptEventType = "error"
	CrawlerPromptEventDone   CrawlerPromptEventType = "done"
)

// CrawlerPromptSource identifies one retrieved chunk the answer may cite.
type CrawlerPromptSource struct {
	ID          int     `json:"id"`
	CrawlerUUID string  `json:"crawler_uuid"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Score       float64 `json:"score"`
}

// CrawlerPromptDone is the terminal frame's payload. Token counts and the model
// id are deliberately absent: the API withholds them from customers because they
// would expose our margin, and the price is the flat APICredit below.
type CrawlerPromptDone struct {
	SourcesUsed []int `json:"sources_used"`
	// SourcesDropped counts retrieved chunks that did not fit the context
	// window. They were ranked in but never shown to the model, so an answer
	// with SourcesDropped > 0 saw less than retrieval found.
	SourcesDropped int `json:"sources_dropped"`
	// Truncated is true when the model hit its output cap. The answer is
	// returned anyway; it is the caller's call whether to use it.
	Truncated bool `json:"truncated"`
	// APICredit is what the engine actually charged, which is not always the
	// list price: a run that produced no answer bills zero. Nil means an engine
	// too old to report it, so fall back to the flat price rather than to zero.
	APICredit *int `json:"api_credit,omitempty"`
}

// CrawlerPromptEvent is one decoded frame of the prompt stream. Exactly one
// of the typed fields is populated, selected by Type.
type CrawlerPromptEvent struct {
	Type CrawlerPromptEventType
	// Token holds the text delta for a token frame.
	Token string
	// Source is populated for a source frame.
	Source CrawlerPromptSource
	// Done is populated for the terminal frame.
	Done CrawlerPromptDone
	// Raw is the frame's undecoded data payload.
	Raw []byte
}

// ==============================================================================
// Auto-refresh (POST/PATCH /crawl/{uuid}/refresh, GET .../refresh/history)
// ==============================================================================

// Refresh interval bounds, in seconds. The floor decides the cost: a crawl
// refreshing every minute re-scrapes the whole site 1,440 times a day.
const (
	CrawlerRefreshMinInterval = 3600
	CrawlerRefreshMaxInterval = 90 * 24 * 3600
)

// Refresh states carried by CrawlerRefreshState.Status.
const (
	CrawlerRefreshDisabled  = "DISABLED"
	CrawlerRefreshScheduled = "SCHEDULED"
	CrawlerRefreshRunning   = "RUNNING"
	CrawlerRefreshFailed    = "FAILED"
)

// CrawlerRefreshEntry is one row of a crawl's refresh timeline.
//
// SampleUpdated/SampleRemoved carry at most ten URLs each. The full lists are
// never inlined: a 5,000-page crawl would otherwise put 5,000 strings into
// every status poll.
type CrawlerRefreshEntry struct {
	At         string `json:"at"`
	Generation int    `json:"generation"`
	// Added are URLs this run discovered that the crawl did not hold; Updated
	// are known URLs whose content fingerprint changed; Removed are known URLs
	// that no longer exist and were dropped.
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
	// Unchanged pages were re-scraped with an identical fingerprint. They cost
	// no embedding and no index write.
	Unchanged int `json:"unchanged"`
	// Failed URLs keep the content they already had.
	Failed        int      `json:"failed"`
	DurationMs    int      `json:"duration_ms"`
	SearchStatus  string   `json:"search_status,omitempty"`
	Error         string   `json:"error,omitempty"`
	SampleUpdated []string `json:"sample_updated,omitempty"`
	SampleRemoved []string `json:"sample_removed,omitempty"`
}

// Changed reports the pages this run actually touched. Zero means the site
// stood still and the run cost no re-indexing.
func (e *CrawlerRefreshEntry) Changed() int {
	if e == nil {
		return 0
	}
	return e.Added + e.Updated + e.Removed
}

// CrawlerRefreshState is a crawl's refresh block. It appears on
// GET /crawl/{uuid}/status and is the answer of the three refresh calls.
//
// One type serves both paths, and the two paths do not carry the same keys:
// /status relays the engine's block verbatim, while the refresh calls render
// the API's own typed state, which declares neither StartedAt nor
// ConsecutiveFailures. Both therefore stay zero-valued on a refresh call and
// must not be read as "the crawl has never run".
type CrawlerRefreshState struct {
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
	Status          string `json:"status"`
	// Generation counts completed refresh runs.
	Generation int    `json:"generation"`
	LastRunAt  string `json:"last_run_at,omitempty"`
	// NextRunAt is empty while refresh is disabled.
	NextRunAt string `json:"next_run_at,omitempty"`
	// StartedAt is set only while a run is in flight, so it is the wall-clock
	// answer to "how long has this been running", which LastRunAt cannot give.
	StartedAt string `json:"started_at,omitempty"`
	Error     string `json:"error,omitempty"`
	// ConsecutiveFailures resets on the first run that succeeds. It is the
	// signal that separates one flaky run from a schedule that has stopped
	// working, which Status alone does not say.
	ConsecutiveFailures int `json:"consecutive_failures"`
	// History is newest-last and capped at the 50 most recent runs.
	History []CrawlerRefreshEntry `json:"history,omitempty"`
}

// IsRunning reports whether a refresh run is in flight right now.
func (r *CrawlerRefreshState) IsRunning() bool {
	return r != nil && r.Status == CrawlerRefreshRunning
}

// LastRun returns the most recent timeline row, nil before the first run.
func (r *CrawlerRefreshState) LastRun() *CrawlerRefreshEntry {
	if r == nil || len(r.History) == 0 {
		return nil
	}
	return &r.History[len(r.History)-1]
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

// CrawlerUpdatedWebhook is the payload for the crawler_updated event, emitted
// once per auto-refresh run that changed at least one page. A run over a site
// that stood still, and a run that failed outright, change nothing and are not
// delivered, so receiving this event is by itself proof of a diff.
type CrawlerUpdatedWebhook struct {
	Event   CrawlerWebhookEvent `json:"event"`
	Payload struct {
		CrawlerWebhookCommon
		SeedURL string `json:"seed_url"`
		// Refresh is the run as the refresh timeline records it, minus the
		// sample lists: this event carries the URLs in Documents instead, at a
		// higher cap.
		Refresh   CrawlerRefreshEntry `json:"refresh"`
		Documents struct {
			// Updated holds the re-indexed URLs, added and changed alike.
			// Which of the two a URL was only survives in the counts.
			Updated []string `json:"updated"`
			Removed []string `json:"removed"`
			// Truncated reports that a list was cut at Scrapfly's 100-URL cap.
			// The counts on Refresh still describe the whole run.
			Truncated bool `json:"truncated"`
		} `json:"documents"`
		Links struct {
			Status string `json:"status"`
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
