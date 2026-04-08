package scrapfly

import (
	"encoding/json"
	"fmt"
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
// Source of truth: apps/scrapfly/scrape-engine/scrape_engine/crawler/webhook_manager.py
// (verified against the public docs and the example payloads shipped in
// apps/scrapfly/web-app/src/Template/Docs/crawler-api/webhooks_example/).
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
)

// IsValid returns true when the event is one of the documented values.
func (e CrawlerWebhookEvent) IsValid() bool {
	switch e {
	case WebhookCrawlerStarted, WebhookCrawlerURLVisited, WebhookCrawlerURLSkipped,
		WebhookCrawlerURLDiscovered, WebhookCrawlerURLFailed, WebhookCrawlerStopped,
		WebhookCrawlerCancelled, WebhookCrawlerFinished:
		return true
	}
	return false
}

// String returns the wire-format value.
func (e CrawlerWebhookEvent) String() string { return string(e) }

// CrawlerConfig configures a Scrapfly Crawler API job.
//
// Every field except URL is optional. Fields left at their zero value are NOT
// sent to the API so the server applies its own documented defaults (e.g.
// respect_robots_txt defaults to true).
//
// Tri-state fields (RespectRobotsTxt, FollowInternalSubdomains) use *bool so
// the SDK can distinguish "unset" from "explicit false". Setting a bool field
// directly (e.g. UseSitemaps=true) is sent as-is; only the tri-state fields
// need pointer semantics.
type CrawlerConfig struct {
	// URL is the starting URL for the crawl (required). Must be HTTP/HTTPS.
	URL string `required:"true"`

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
	FollowInternalSubdomains *bool
	AllowedInternalSubdomains []string

	// Request configuration.
	Headers         map[string]string
	Delay           int // ms, 0-15000
	UserAgent       string
	MaxConcurrency  int
	RenderingDelay  int // ms, 0-25000

	// Crawl strategy.
	UseSitemaps   bool
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

	// Web scraping features.
	ASP       bool
	ProxyPool string
	Country   string

	// Webhook integration.
	WebhookName   string
	WebhookEvents []CrawlerWebhookEvent `validate:"enum"`
}

// toJSONBody serializes the config into a JSON body for POST /crawl.
//
// Unlike ScrapeConfig.toAPIParams (which builds URL query parameters), the
// Crawler API takes its config as a JSON body. The API key is NOT included
// here — it's added to the URL query string by the client method.
//
// Zero-valued fields are dropped so the server applies its own defaults.
func (c *CrawlerConfig) toJSONBody() ([]byte, error) {
	if err := ValidateRequiredFields(c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCrawlerConfig, err)
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

	body := map[string]interface{}{
		"url": c.URL,
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
	if c.ASP {
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

	return json.Marshal(body)
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
