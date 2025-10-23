package scrapfly

import (
	"errors"
	"fmt"
)

// Custom error types for the Scrapfly client
var (
	ErrBadAPIKey            = errors.New("invalid key, must be a non-empty string")
	ErrScrapeConfig         = errors.New("invalid scrape config")
	ErrScreenshotConfig     = errors.New("invalid screenshot config")
	ErrExtractionConfig     = errors.New("invalid extraction config")
	ErrContentType          = errors.New("invalid content type for this operation")
	ErrTooManyRequests      = errors.New("too many requests")
	ErrQuotaLimitReached    = errors.New("quota limit reached")
	ErrScreenshotAPIFailed  = errors.New("screenshot API error")
	ErrExtractionAPIFailed  = errors.New("extraction API error")
	ErrUpstreamClient       = errors.New("upstream http client error")
	ErrUpstreamServer       = errors.New("upstream http server error")
	ErrAPIClient            = errors.New("API http client error")
	ErrAPIServer            = errors.New("API http server error")
	ErrScrapeFailed         = errors.New("scrape failed")
	ErrProxyFailed          = errors.New("proxy error")
	ErrASPBypassFailed      = errors.New("ASP bypass error")
	ErrScheduleFailed       = errors.New("schedule error")
	ErrWebhookFailed        = errors.New("webhook error")
	ErrSessionFailed        = errors.New("session error")
	ErrUnhandledAPIResponse = errors.New("unhandled API error response")
)

// APIError represents an error returned by the Scrapfly API.
type APIError struct {
	Message          string
	Code             string
	HTTPStatusCode   int
	DocumentationURL string
	APIResponse      *ScrapeResult
	RetryAfterMs     int
	Hint             string
}
func (e *APIError) Error() string {
	base := fmt.Sprintf("API Error: %s (code: %s, status: %d, docs: %s)", e.Message, e.Code, e.HTTPStatusCode, e.DocumentationURL)
	if e.RetryAfterMs > 0 {
		base += fmt.Sprintf(", retry_after_ms: %d", e.RetryAfterMs)
	}
	return base
}
