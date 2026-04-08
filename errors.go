package scrapfly

import (
	"errors"
	"fmt"
)

// Sentinel errors for the Scrapfly client.
// These errors can be used with errors.Is() for error checking.
var (
	// ErrBadAPIKey indicates an invalid or empty API key was provided.
	ErrBadAPIKey = errors.New("invalid key, must be a non-empty string")

	// ErrScrapeConfig indicates invalid scrape configuration.
	ErrScrapeConfig = errors.New("invalid scrape config")

	// ErrScreenshotConfig indicates invalid screenshot configuration.
	ErrScreenshotConfig = errors.New("invalid screenshot config")

	// ErrExtractionConfig indicates invalid extraction configuration.
	ErrExtractionConfig = errors.New("invalid extraction config")

	// ErrContentType indicates an invalid content type for the requested operation.
	ErrContentType = errors.New("invalid content type for this operation")

	// ErrTooManyRequests indicates rate limiting is in effect.
	ErrTooManyRequests = errors.New("too many requests")

	// ErrQuotaLimitReached indicates the account quota has been exceeded.
	ErrQuotaLimitReached = errors.New("quota limit reached")

	// ErrScreenshotAPIFailed indicates a screenshot API error occurred.
	ErrScreenshotAPIFailed = errors.New("screenshot API error")

	// ErrExtractionAPIFailed indicates an extraction API error occurred.
	ErrExtractionAPIFailed = errors.New("extraction API error")

	// ErrUpstreamClient indicates a 4xx error from the target website.
	ErrUpstreamClient = errors.New("upstream http client error")

	// ErrUpstreamServer indicates a 5xx error from the target website.
	ErrUpstreamServer = errors.New("upstream http server error")

	// ErrAPIClient indicates a 4xx error from the Scrapfly API.
	ErrAPIClient = errors.New("API http client error")

	// ErrAPIServer indicates a 5xx error from the Scrapfly API.
	ErrAPIServer = errors.New("API http server error")

	// ErrScrapeFailed indicates the scraping operation failed.
	ErrScrapeFailed = errors.New("scrape failed")

	// ErrProxyFailed indicates a proxy connection error.
	ErrProxyFailed = errors.New("proxy error")

	// ErrASPBypassFailed indicates Anti-Scraping Protection bypass failed.
	ErrASPBypassFailed = errors.New("ASP bypass error")

	// ErrScheduleFailed indicates a scheduled job error.
	ErrScheduleFailed = errors.New("schedule error")

	// ErrWebhookFailed indicates a webhook delivery error.
	ErrWebhookFailed = errors.New("webhook error")

	// ErrSessionFailed indicates a browser session error.
	ErrSessionFailed = errors.New("session error")

	// ErrUnhandledAPIResponse indicates an unexpected API error response.
	ErrUnhandledAPIResponse = errors.New("unhandled API error response")

	// ErrCrawlerConfig indicates invalid crawler configuration.
	ErrCrawlerConfig = errors.New("invalid crawler config")

	// ErrCrawlerFailed indicates a crawler API error (e.g. ERR::CRAWLER::*).
	ErrCrawlerFailed = errors.New("crawler error")

	// ErrCrawlerNotStarted indicates Crawl helper methods were called before Start().
	ErrCrawlerNotStarted = errors.New("crawler not started, call Start() first")

	// ErrCrawlerAlreadyStarted indicates Crawl.Start() was called twice.
	ErrCrawlerAlreadyStarted = errors.New("crawler already started")

	// ErrCrawlerTimeout indicates Crawl.Wait() exceeded the caller's deadline.
	ErrCrawlerTimeout = errors.New("crawler wait timed out")

	// ErrCrawlerCancelled indicates Crawl.Wait() observed a CANCELLED terminal state.
	ErrCrawlerCancelled = errors.New("crawler was cancelled")

	// ErrUnexpectedResponseFormat indicates the server returned a Content-Type the SDK didn't expect.
	// Used for example when GET /crawl/{uuid}/urls returns JSON instead of streaming text.
	ErrUnexpectedResponseFormat = errors.New("unexpected response format")
)

// APIError represents a detailed error returned by the Scrapfly API.
//
// It contains information about the error including status codes, error messages,
// documentation links, and retry information.
type APIError struct {
	// Message is the human-readable error message.
	Message string
	// Code is the error code identifier.
	Code string
	// HTTPStatusCode is the HTTP status code of the response.
	HTTPStatusCode int
	// DocumentationURL provides a link to relevant documentation.
	DocumentationURL string
	// APIResponse contains the full API response (if available).
	APIResponse *ScrapeResult
	// RetryAfterMs indicates how long to wait before retrying (for rate limits).
	RetryAfterMs int
	// Hint provides additional context or suggestions for resolving the error.
	Hint string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	base := fmt.Sprintf("API Error: %s (code: %s, status: %d, docs: %s)", e.Message, e.Code, e.HTTPStatusCode, e.DocumentationURL)
	if e.RetryAfterMs > 0 {
		base += fmt.Sprintf(", retry_after_ms: %d", e.RetryAfterMs)
	}
	return base
}
