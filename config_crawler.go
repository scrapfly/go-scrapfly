package scrapfly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// CrawlerContentFormat represents the content format to extract from crawled pages.
//
// Corresponds to the `content_formats` array on POST /crawl. The server-side
// contract permits any subset of these formats per crawler config.
type CrawlerContentFormat string

// Content formats supported by the Crawler API.
const (
	CrawlerFormatHTML          CrawlerContentFormat = "html"
	CrawlerFormatCleanHTML     CrawlerContentFormat = "clean_html"
	CrawlerFormatMarkdown      CrawlerContentFormat = "markdown"
	CrawlerFormatText          CrawlerContentFormat = "text"
	CrawlerFormatJSON          CrawlerContentFormat = "json"
	CrawlerFormatExtractedData CrawlerContentFormat = "extracted_data"
	CrawlerFormatPageMetadata  CrawlerContentFormat = "page_metadata"
)

// IsValid returns true when the format is one of the documented values.
// Used by the SDK's reflective enum validator.
func (f CrawlerContentFormat) IsValid() bool {
	switch f {
	case CrawlerFormatHTML, CrawlerFormatCleanHTML, CrawlerFormatMarkdown,
		CrawlerFormatText, CrawlerFormatJSON, CrawlerFormatExtractedData,
		CrawlerFormatPageMetadata:
		return true
	}
	return false
}

// String returns the wire-format value.
func (f CrawlerContentFormat) String() string { return string(f) }

// CrawlerWebhookEvent enumerates the webhook events the crawler can emit.
//
// Event names match the wire format documented in the Crawler API webhook
// reference, and are verified against its example payloads.
type CrawlerWebhookEvent string

// Crawler webhook event names.
const (
	WebhookCrawlerStarted       CrawlerWebhookEvent = "crawler_started"
	WebhookCrawlerURLVisited    CrawlerWebhookEvent = "crawler_url_visited"
	WebhookCrawlerURLSkipped    CrawlerWebhookEvent = "crawler_url_skipped"
	WebhookCrawlerURLDiscovered CrawlerWebhookEvent = "crawler_url_discovered"
	WebhookCrawlerURLFailed     CrawlerWebhookEvent = "crawler_url_failed"
	WebhookCrawlerStopped       CrawlerWebhookEvent = "crawler_stopped"
	WebhookCrawlerCancelled     CrawlerWebhookEvent = "crawler_cancelled"
	WebhookCrawlerFinished      CrawlerWebhookEvent = "crawler_finished"
	WebhookCrawlerSearchReady   CrawlerWebhookEvent = "crawler_search_ready"
	WebhookCrawlerSearchFailed  CrawlerWebhookEvent = "crawler_search_failed"
	WebhookCrawlerUpdated       CrawlerWebhookEvent = "crawler_updated"
)

// IsValid returns true when the event is one of the documented values.
func (e CrawlerWebhookEvent) IsValid() bool {
	switch e {
	case WebhookCrawlerStarted, WebhookCrawlerURLVisited, WebhookCrawlerURLSkipped,
		WebhookCrawlerURLDiscovered, WebhookCrawlerURLFailed, WebhookCrawlerStopped,
		WebhookCrawlerCancelled, WebhookCrawlerFinished,
		WebhookCrawlerSearchReady, WebhookCrawlerSearchFailed, WebhookCrawlerUpdated:
		return true
	}
	return false
}

// String returns the wire-format value.
func (e CrawlerWebhookEvent) String() string { return string(e) }

// CrawlerConfig configures a Scrapfly Crawler API job.
//
// Exactly one URL source must be provided: URL (seed crawl with discovery),
// URLList (explicit in-memory list, no discovery), or RemoteURLList (URL of
// a hosted text file fetched at crawl start, no discovery). Other fields are
// optional and default to server-side values when zero-valued.
//
// Tri-state fields (RespectRobotsTxt, FollowInternalSubdomains, Unblocker) use
// *bool so the SDK can distinguish "unset" from "explicit false". Setting a
// bool field directly (e.g. UseSitemaps=true) is sent as-is; only the tri-state
// fields need pointer semantics.
type CrawlerConfig struct {
	// URL source — exactly one of URL, URLList, RemoteURLList. URL enables
	// discovery (sitemaps, robots.txt, link-following); URLList and
	// RemoteURLList crawl exactly the URLs they reference, no discovery.
	URL           string
	URLList       []string
	RemoteURLList string

	// Crawl limits. Zero means "unset" — the server applies its own default.
	PageLimit    int
	MaxDepth     int
	MaxDuration  int // seconds, 15-10800
	MaxAPICredit int // 0 means no limit per the docs

	// Path filtering (mutually exclusive — validated at serialization time).
	ExcludePaths     []string `exclusive:"path_filter"`
	IncludeOnlyPaths []string `exclusive:"path_filter"`

	// Domain & subdomain restrictions.
	IgnoreBasePathRestriction bool
	FollowExternalLinks       bool
	AllowedExternalDomains    []string

	// Tri-state. nil = unset (server default True); non-nil = explicit override.
	FollowInternalSubdomains  *bool
	AllowedInternalSubdomains []string

	// Request configuration.
	Headers        map[string]string
	Delay          int // ms, 0-15000
	UserAgent      string
	MaxConcurrency int
	RenderingDelay int // ms, 0-25000

	// Crawl strategy.
	UseSitemaps    bool
	IgnoreNoFollow bool

	// Tri-state. nil = unset (server default True); non-nil = explicit override.
	RespectRobotsTxt *bool

	// Cache.
	Cache      bool
	CacheTTL   int // seconds, 0-604800
	CacheClear bool

	// Content extraction.
	ContentFormats  []CrawlerContentFormat `validate:"enum"`
	ExtractionRules map[string]interface{}

	// Search index built while the crawl runs. Query it with CrawlSearch /
	// CrawlPrompt once the index reaches READY.
	Search bool

	// Refresh keeps this crawl fresh: its own URLs are re-scraped in place on
	// a period, under the same crawler UUID and the same artifacts. Only pages
	// whose content changed are re-indexed and pages that disappeared are
	// dropped.
	Refresh bool
	// RefreshInterval is the period in seconds, CrawlerRefreshMinInterval to
	// CrawlerRefreshMaxInterval. Zero leaves the server default period.
	RefreshInterval int

	// Web scraping features.

	// Unblocker enables the anti-bot bypass (formerly "ASP").
	// nil means unset; set it with BoolPtr(true) / BoolPtr(false).
	// Serialized into the POST /crawl body under the "asp" key — the wire key
	// is unchanged.
	// See resolveUnblocker for the ASP/Unblocker precedence rule.
	//
	// The two names are two INDEPENDENT fields, not an aliased pair: writing
	// one never updates the other, so reading Unblocker after setting ASP
	// returns nil, and ASP: true wins no matter which was written last. To turn
	// the feature off, clear ASP — setting Unblocker to BoolPtr(false) does not
	// override an ASP: true. Use UnblockerEnabled to read what will actually go
	// on the wire. The other Scrapfly SDKs expose one storage slot behind two
	// names, so this and the ASP: false + Unblocker: BoolPtr(true) row are the
	// two places Go answers differently; both follow from ASP being a plain
	// bool, which cannot be retyped without breaking every existing caller.
	Unblocker *bool
	// ASP enables Anti-Scraping Protection bypass.
	//
	// Deprecated: use Unblocker. ASP keeps working forever; it is only the
	// documented name that changed. When ASP is true it wins over Unblocker,
	// because a plain bool cannot distinguish "unset" from "explicitly false"
	// and honouring it is the only way an old caller keeps its feature.
	ASP       bool
	ProxyPool string
	Country   string

	// Webhook integration.
	WebhookName   string
	WebhookEvents []CrawlerWebhookEvent `validate:"enum"`
}

// validateUrlSource enforces the URL-source mutex: exactly one of URL,
// URLList, or RemoteURLList must be set. Returns ErrCrawlerConfig wrapping
// a human-readable error.
func (c *CrawlerConfig) validateUrlSource() error {
	hasSeed := c.URL != ""
	hasList := len(c.URLList) > 0
	hasRemote := c.RemoteURLList != ""
	count := 0
	if hasSeed {
		count++
	}
	if hasList {
		count++
	}
	if hasRemote {
		count++
	}
	if count == 0 {
		return fmt.Errorf("%w: provide one of URL, URLList, or RemoteURLList", ErrCrawlerConfig)
	}
	if count > 1 {
		return fmt.Errorf("%w: only one of URL, URLList, or RemoteURLList can be set", ErrCrawlerConfig)
	}
	return nil
}

// buildBodyMap assembles the crawler config as a map. URLList is omitted —
// callers that need it serialize the list separately (in-memory list goes
// out as a multipart 'urls' part; otherwise the JSON path inlines it).
func (c *CrawlerConfig) buildBodyMap() map[string]interface{} {
	body := map[string]interface{}{}
	if c.URL != "" {
		body["url"] = c.URL
	}
	if c.RemoteURLList != "" {
		body["remote_url_list"] = c.RemoteURLList
	}
	if c.PageLimit != 0 {
		body["page_limit"] = c.PageLimit
	}
	if c.MaxDepth != 0 {
		body["max_depth"] = c.MaxDepth
	}
	if c.MaxDuration != 0 {
		body["max_duration"] = c.MaxDuration
	}
	if c.MaxAPICredit != 0 {
		body["max_api_credit"] = c.MaxAPICredit
	}
	if len(c.ExcludePaths) > 0 {
		body["exclude_paths"] = c.ExcludePaths
	}
	if len(c.IncludeOnlyPaths) > 0 {
		body["include_only_paths"] = c.IncludeOnlyPaths
	}
	if c.IgnoreBasePathRestriction {
		body["ignore_base_path_restriction"] = true
	}
	if c.FollowExternalLinks {
		body["follow_external_links"] = true
	}
	if len(c.AllowedExternalDomains) > 0 {
		body["allowed_external_domains"] = c.AllowedExternalDomains
	}
	// Tri-state: only serialize if explicitly set.
	if c.FollowInternalSubdomains != nil {
		body["follow_internal_subdomains"] = *c.FollowInternalSubdomains
	}
	if len(c.AllowedInternalSubdomains) > 0 {
		body["allowed_internal_subdomains"] = c.AllowedInternalSubdomains
	}
	if len(c.Headers) > 0 {
		body["headers"] = c.Headers
	}
	if c.Delay != 0 {
		body["delay"] = c.Delay
	}
	if c.UserAgent != "" {
		body["user_agent"] = c.UserAgent
	}
	if c.MaxConcurrency != 0 {
		body["max_concurrency"] = c.MaxConcurrency
	}
	if c.RenderingDelay != 0 {
		body["rendering_delay"] = c.RenderingDelay
	}
	if c.UseSitemaps {
		body["use_sitemaps"] = true
	}
	// Tri-state: only serialize if explicitly set.
	if c.RespectRobotsTxt != nil {
		body["respect_robots_txt"] = *c.RespectRobotsTxt
	}
	if c.IgnoreNoFollow {
		body["ignore_no_follow"] = true
	}
	if c.Cache {
		body["cache"] = true
	}
	if c.CacheTTL != 0 {
		body["cache_ttl"] = c.CacheTTL
	}
	if c.CacheClear {
		body["cache_clear"] = true
	}
	if len(c.ContentFormats) > 0 {
		formats := make([]string, len(c.ContentFormats))
		for i, f := range c.ContentFormats {
			formats[i] = string(f)
		}
		body["content_formats"] = formats
	}
	if len(c.ExtractionRules) > 0 {
		body["extraction_rules"] = c.ExtractionRules
	}
	if c.Search {
		body["search"] = true
	}
	if c.Refresh {
		body["refresh"] = true
	}
	if c.RefreshInterval != 0 {
		body["refresh_interval"] = c.RefreshInterval
	}
	// Wire key stays "asp" for both ASP and Unblocker — see resolveUnblocker.
	if resolveUnblocker(c.ASP, c.Unblocker) {
		body["asp"] = true
	}
	if c.ProxyPool != "" {
		body["proxy_pool"] = c.ProxyPool
	}
	if c.Country != "" {
		body["country"] = c.Country
	}
	if c.WebhookName != "" {
		body["webhook_name"] = c.WebhookName
	}
	if len(c.WebhookEvents) > 0 {
		events := make([]string, len(c.WebhookEvents))
		for i, e := range c.WebhookEvents {
			events[i] = string(e)
		}
		body["webhook_events"] = events
	}

	return body
}

// toJSONBody serializes the config into a JSON body for POST /crawl. Used
// for seed-URL crawls and remote_url_list crawls — for in-memory URL lists
// the SDK switches to multipart (see toMultipartBody).
//
// Zero-valued fields are dropped so the server applies its own defaults.
func (c *CrawlerConfig) toJSONBody() ([]byte, error) {
	if err := c.validateUrlSource(); err != nil {
		return nil, err
	}
	if err := ValidateExclusiveFields(c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCrawlerConfig, err)
	}
	if err := ValidateEnums(c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCrawlerConfig, err)
	}
	if err := c.validateBounds(); err != nil {
		return nil, err
	}

	body := c.buildBodyMap()
	if len(c.URLList) > 0 {
		body["url_list"] = c.URLList
	}
	return json.Marshal(body)
}

// toMultipartBody builds a multipart/form-data body for POST /crawl when an
// in-memory URLList is supplied. The 'config' part carries the JSON config
// (without url_list) and the 'urls' part carries the URLs as text/plain,
// one per line. Returns the body bytes and the matching Content-Type
// header (with the boundary baked in).
func (c *CrawlerConfig) toMultipartBody() ([]byte, string, error) {
	if err := c.validateUrlSource(); err != nil {
		return nil, "", err
	}
	if err := ValidateExclusiveFields(c); err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrCrawlerConfig, err)
	}
	if err := ValidateEnums(c); err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrCrawlerConfig, err)
	}
	if err := c.validateBounds(); err != nil {
		return nil, "", err
	}
	if len(c.URLList) == 0 {
		return nil, "", fmt.Errorf("%w: toMultipartBody requires URLList to be set", ErrCrawlerConfig)
	}

	configJSON, err := json.Marshal(c.buildBodyMap())
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	cfgHdr := make(textproto.MIMEHeader)
	cfgHdr.Set("Content-Disposition", `form-data; name="config"; filename="config.json"`)
	cfgHdr.Set("Content-Type", "application/json")
	cfgPart, err := mw.CreatePart(cfgHdr)
	if err != nil {
		return nil, "", err
	}
	if _, err := cfgPart.Write(configJSON); err != nil {
		return nil, "", err
	}

	urlsHdr := make(textproto.MIMEHeader)
	urlsHdr.Set("Content-Disposition", `form-data; name="urls"; filename="urls.txt"`)
	urlsHdr.Set("Content-Type", "text/plain")
	urlsPart, err := mw.CreatePart(urlsHdr)
	if err != nil {
		return nil, "", err
	}
	if _, err := urlsPart.Write([]byte(strings.Join(c.URLList, "\n"))); err != nil {
		return nil, "", err
	}

	if err := mw.Close(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), mw.FormDataContentType(), nil
}

// validateBounds enforces the numeric bounds documented in the public API.
// Called from toJSONBody; returns an error wrapping ErrCrawlerConfig if any
// field is out of range.
func (c *CrawlerConfig) validateBounds() error {
	if c.PageLimit < 0 {
		return fmt.Errorf("%w: page_limit must be >= 0 (0 = unlimited), got %d", ErrCrawlerConfig, c.PageLimit)
	}
	if c.MaxDepth < 0 {
		return fmt.Errorf("%w: max_depth must be >= 0, got %d", ErrCrawlerConfig, c.MaxDepth)
	}
	if c.RenderingDelay < 0 || c.RenderingDelay > 25000 {
		return fmt.Errorf("%w: rendering_delay must be between 0 and 25000 ms, got %d", ErrCrawlerConfig, c.RenderingDelay)
	}
	if c.Delay < 0 || c.Delay > 15000 {
		return fmt.Errorf("%w: delay must be between 0 and 15000 ms, got %d", ErrCrawlerConfig, c.Delay)
	}
	if c.CacheTTL < 0 || c.CacheTTL > 604800 {
		return fmt.Errorf("%w: cache_ttl must be between 0 and 604800 seconds, got %d", ErrCrawlerConfig, c.CacheTTL)
	}
	// max_duration: server accepts 0 as "unset / use default" — only enforce bounds when non-zero.
	if c.MaxDuration != 0 && (c.MaxDuration < 15 || c.MaxDuration > 10800) {
		return fmt.Errorf("%w: max_duration must be between 15 and 10800 seconds, got %d", ErrCrawlerConfig, c.MaxDuration)
	}
	if c.MaxAPICredit < 0 {
		return fmt.Errorf("%w: max_api_credit must be >= 0 (0 = no limit), got %d", ErrCrawlerConfig, c.MaxAPICredit)
	}
	if len(c.ExcludePaths) > 100 {
		return fmt.Errorf("%w: exclude_paths is limited to 100 entries, got %d", ErrCrawlerConfig, len(c.ExcludePaths))
	}
	if len(c.IncludeOnlyPaths) > 100 {
		return fmt.Errorf("%w: include_only_paths is limited to 100 entries, got %d", ErrCrawlerConfig, len(c.IncludeOnlyPaths))
	}
	if len(c.AllowedExternalDomains) > 250 {
		return fmt.Errorf("%w: allowed_external_domains is limited to 250 entries, got %d", ErrCrawlerConfig, len(c.AllowedExternalDomains))
	}
	if len(c.AllowedInternalSubdomains) > 250 {
		return fmt.Errorf("%w: allowed_internal_subdomains is limited to 250 entries, got %d", ErrCrawlerConfig, len(c.AllowedInternalSubdomains))
	}
	// refresh_interval: zero means "unset / use default" — only enforce bounds
	// when non-zero. The floor decides the cost, a crawl refreshing every
	// minute re-scrapes the whole site 1,440 times a day.
	if c.RefreshInterval != 0 && (c.RefreshInterval < CrawlerRefreshMinInterval || c.RefreshInterval > CrawlerRefreshMaxInterval) {
		return fmt.Errorf("%w: refresh_interval must be between %d and %d seconds, got %d", ErrCrawlerConfig, CrawlerRefreshMinInterval, CrawlerRefreshMaxInterval, c.RefreshInterval)
	}
	if c.RefreshInterval != 0 && !c.Refresh {
		return fmt.Errorf("%w: refresh_interval requires Refresh=true", ErrCrawlerConfig)
	}
	return nil
}

// BoolPtr is a tiny helper for constructing *bool tri-state fields.
//
//	config := &scrapfly.CrawlerConfig{
//	    URL:               "https://example.com",
//	    RespectRobotsTxt:  scrapfly.BoolPtr(false),
//	    FollowInternalSubdomains: scrapfly.BoolPtr(true),
//	}
func BoolPtr(v bool) *bool { return &v }
