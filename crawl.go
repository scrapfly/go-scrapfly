package scrapfly

import (
	"errors"
	"fmt"
	"time"
)

// Crawl is the high-level fluent wrapper around a single crawler job lifecycle.
//
// It tracks the UUID returned by Start() and caches the most recent status
// and downloaded artifacts. Callers can use it as:
//
//	crawl := scrapfly.NewCrawl(client, &scrapfly.CrawlerConfig{
//	    URL:       "https://web-scraping.dev/products",
//	    PageLimit: 10,
//	})
//	if err := crawl.Start(); err != nil { log.Fatal(err) }
//	if err := crawl.Wait(nil); err != nil { log.Fatal(err) }
//
//	status, _ := crawl.Status(true)
//	fmt.Println("visited", status.State.URLsVisited)
//
//	md, _ := crawl.Read("https://web-scraping.dev/products", scrapfly.CrawlerFormatMarkdown)
//	fmt.Println(md[:200])
//
// Use the lower-level client methods (Client.StartCrawl, Client.CrawlStatus,
// etc.) directly if you need finer control, concurrency, or want to avoid
// the caching behavior.
type Crawl struct {
	client *Client
	config *CrawlerConfig

	uuid            string
	cachedStatus    *CrawlerStatus
	cachedArtifacts map[CrawlerArtifactType]*CrawlerArtifact
}

// NewCrawl constructs a Crawl wrapper. It does NOT schedule the job — call
// Start() to actually submit the crawler config to the API.
func NewCrawl(client *Client, config *CrawlerConfig) *Crawl {
	return &Crawl{
		client:          client,
		config:          config,
		cachedArtifacts: make(map[CrawlerArtifactType]*CrawlerArtifact),
	}
}

// UUID returns the crawler job UUID. Empty before Start() is called.
func (c *Crawl) UUID() string { return c.uuid }

// Started reports whether Start() has been called successfully.
func (c *Crawl) Started() bool { return c.uuid != "" }

// Start schedules the crawl on the Scrapfly API. Calling it twice returns
// ErrCrawlerAlreadyStarted.
func (c *Crawl) Start() error {
	if c.uuid != "" {
		return ErrCrawlerAlreadyStarted
	}
	resp, err := c.client.StartCrawl(c.config)
	if err != nil {
		return err
	}
	c.uuid = resp.CrawlerUUID
	return nil
}

// requireStarted is a shared guard for methods that only make sense after Start().
func (c *Crawl) requireStarted() error {
	if c.uuid == "" {
		return ErrCrawlerNotStarted
	}
	return nil
}

// Status fetches the current status of the crawler job.
//
// When refresh=true, always hits the API. When refresh=false, returns the
// most recently cached status (or hits the API on first call). Most callers
// want refresh=true inside polling loops; set refresh=false when you just
// need to re-read the last known state cheaply.
func (c *Crawl) Status(refresh bool) (*CrawlerStatus, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	if refresh || c.cachedStatus == nil {
		status, err := c.client.CrawlStatus(c.uuid)
		if err != nil {
			return nil, err
		}
		c.cachedStatus = status
	}
	return c.cachedStatus, nil
}

// WaitOptions configures the polling behavior of Crawl.Wait.
type WaitOptions struct {
	// PollInterval controls how often to hit the status endpoint.
	// Defaults to 5 seconds when zero.
	PollInterval time.Duration
	// MaxWait is the total wall-clock deadline for the wait loop.
	// Zero means "no timeout" — wait indefinitely.
	MaxWait time.Duration
	// Verbose enables per-poll progress logging via the default SDK logger.
	Verbose bool
	// AllowCancelled, when true, makes Wait return nil instead of
	// ErrCrawlerCancelled when the crawler reaches the CANCELLED terminal
	// state. Useful for the cancel-then-wait pattern where the caller
	// already triggered the cancellation themselves. Defaults to false,
	// preserving the prior return-error behavior for callers that observe
	// external interrupts.
	AllowCancelled bool
}

// Wait polls Status until the crawler reaches a terminal state.
//
// Returns nil when the crawler reaches DONE with IsSuccess=true.
// Returns an error wrapping ErrCrawlerFailed / ErrCrawlerCancelled / ErrCrawlerTimeout
// for the other terminal conditions.
//
// Pass nil for default behavior (5-second polling, no timeout).
func (c *Crawl) Wait(opts *WaitOptions) error {
	if err := c.requireStarted(); err != nil {
		return err
	}
	if opts == nil {
		opts = &WaitOptions{}
	}
	interval := opts.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	var deadline time.Time
	if opts.MaxWait > 0 {
		deadline = time.Now().Add(opts.MaxWait)
	}

	for {
		status, err := c.Status(true)
		if err != nil {
			return err
		}
		if opts.Verbose {
			DefaultLogger.Info(
				"crawl progress",
				"uuid", c.uuid,
				"status", status.Status,
				"visited", status.State.URLsVisited,
				"extracted", status.State.URLsExtracted,
			)
		}
		if status.IsFinished || status.Status == CrawlerStatusCancelled {
			if status.IsFailed() {
				stopReason := ""
				if status.State.StopReason != nil {
					stopReason = *status.State.StopReason
				}
				return fmt.Errorf("%w: crawl %s failed (stop_reason=%s)", ErrCrawlerFailed, c.uuid, stopReason)
			}
			if status.IsCancelled() {
				if opts.AllowCancelled {
					if opts.Verbose {
						DefaultLogger.Info("crawl was cancelled (AllowCancelled=true)", "uuid", c.uuid)
					}
					return nil
				}
				return fmt.Errorf("%w: crawl %s was cancelled", ErrCrawlerCancelled, c.uuid)
			}
			return nil
		}

		// Timeout check BEFORE sleeping so we don't overshoot by one interval.
		if !deadline.IsZero() && time.Now().Add(interval).After(deadline) {
			return fmt.Errorf("%w: crawl %s did not finish within %s", ErrCrawlerTimeout, c.uuid, opts.MaxWait)
		}
		time.Sleep(interval)
	}
}

// Cancel cancels this crawl. Returns nil on success. Calling Cancel on a
// crawler that has already finished is a server-side no-op (still returns nil).
//
// Mirrors Python SDK's `Crawl.cancel()`.
func (c *Crawl) Cancel() error {
	if err := c.requireStarted(); err != nil {
		return err
	}
	return c.client.CrawlCancel(c.uuid)
}

// URLs lists crawled URLs for this crawl. Thin wrapper around Client.CrawlURLs.
func (c *Crawl) URLs(opts *CrawlURLsOptions) (*CrawlerURLs, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	return c.client.CrawlURLs(c.uuid, opts)
}

// Read fetches the content for a single crawled URL.
//
// Returns a CrawlContent wrapper for parity with the Python SDK's `Crawl.read()`.
// The wrapper carries the raw content string plus the originating URL and
// crawler UUID. Status code / headers / duration are not populated by the
// plain-mode endpoint (those would require fetching the full WARC artifact).
//
// Returns nil if the URL was not part of this crawl (server returns 404).
func (c *Crawl) Read(targetURL string, format CrawlerContentFormat) (*CrawlContent, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	body, err := c.client.CrawlContentsPlain(c.uuid, targetURL, format)
	if err != nil {
		// 404 is "URL not found in this crawl" — return nil rather than error,
		// matching the Python SDK convention.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}
	return &CrawlContent{
		URL:       targetURL,
		Content:   body,
		CrawlUUID: c.uuid,
	}, nil
}

// ReadString is a convenience wrapper for callers that just want the raw
// content string without the CrawlContent envelope. Returns an empty string
// (and nil error) when the URL isn't part of this crawl.
func (c *Crawl) ReadString(targetURL string, format CrawlerContentFormat) (string, error) {
	content, err := c.Read(targetURL, format)
	if err != nil || content == nil {
		return "", err
	}
	return content.Content, nil
}

// ReadBatch retrieves content for up to 100 URLs in one round-trip.
func (c *Crawl) ReadBatch(urls []string, formats []CrawlerContentFormat) (map[string]map[string]string, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	return c.client.CrawlContentsBatch(c.uuid, urls, formats)
}

// Contents returns the bulk JSON envelope for crawled pages.
func (c *Crawl) Contents(format CrawlerContentFormat, opts *CrawlContentsOptions) (*CrawlerContents, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	return c.client.CrawlContentsJSON(c.uuid, format, opts)
}

// WARC downloads the WARC artifact for this crawl (cached after first call).
func (c *Crawl) WARC() (*CrawlerArtifact, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	if cached, ok := c.cachedArtifacts[ArtifactTypeWARC]; ok {
		return cached, nil
	}
	artifact, err := c.client.CrawlArtifact(c.uuid, ArtifactTypeWARC)
	if err != nil {
		return nil, err
	}
	c.cachedArtifacts[ArtifactTypeWARC] = artifact
	return artifact, nil
}

// HAR downloads the HAR artifact for this crawl (cached after first call).
func (c *Crawl) HAR() (*CrawlerArtifact, error) {
	if err := c.requireStarted(); err != nil {
		return nil, err
	}
	if cached, ok := c.cachedArtifacts[ArtifactTypeHAR]; ok {
		return cached, nil
	}
	artifact, err := c.client.CrawlArtifact(c.uuid, ArtifactTypeHAR)
	if err != nil {
		return nil, err
	}
	c.cachedArtifacts[ArtifactTypeHAR] = artifact
	return artifact, nil
}
