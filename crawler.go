package scrapfly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ==============================================================================
// Crawler API client methods
// ==============================================================================

// StartCrawl schedules a new crawler job and returns the crawler UUID.
//
// This corresponds to `POST /crawl`. The config is serialized as a JSON body
// (not URL query params like Scrape) because the Crawler API takes a richer
// structured config. The API key is still sent via the `key` query parameter.
//
// Example:
//
//	config := &scrapfly.CrawlerConfig{
//	    URL:            "https://web-scraping.dev/products",
//	    PageLimit:      10,
//	    ContentFormats: []scrapfly.CrawlerContentFormat{scrapfly.CrawlerFormatMarkdown},
//	}
//	start, err := client.StartCrawl(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("uuid:", start.CrawlerUUID)
func (c *Client) StartCrawl(config *CrawlerConfig) (*CrawlerStartResponse, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: config is nil", ErrCrawlerConfig)
	}

	body, err := config.toJSONBody()
	if err != nil {
		return nil, err
	}

	endpointURL, _ := url.Parse(c.host + "/crawl")
	q := url.Values{}
	q.Set("key", c.key)
	endpointURL.RawQuery = q.Encode()

	req, err := http.NewRequest("POST", endpointURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	// GetBody enables retry: fetchWithRetry re-reads the body on each attempt.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
	}

	var out CrawlerStartResponse
	if err := json.Unmarshal(bodyBytes, &out); err != nil {
		return nil, fmt.Errorf("failed to decode crawler start response: %w", err)
	}
	if out.CrawlerUUID == "" {
		return nil, fmt.Errorf("crawler start response missing crawler_uuid: %s", string(bodyBytes))
	}
	return &out, nil
}

// CrawlStatus fetches the current status of a crawler job.
//
// `GET /crawl/{uuid}/status` — returns a strictly-validated CrawlerStatus.
// The caller can use IsRunning/IsComplete/IsFailed/IsCancelled to check state
// transitions.
func (c *Client) CrawlStatus(uuid string) (*CrawlerStatus, error) {
	if uuid == "" {
		return nil, fmt.Errorf("%w: uuid must be a non-empty string", ErrCrawlerConfig)
	}

	endpointURL, _ := url.Parse(c.host + "/crawl/" + url.PathEscape(uuid) + "/status")
	q := url.Values{}
	q.Set("key", c.key)
	endpointURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
	}

	return parseCrawlerStatus(bodyBytes)
}

// CrawlURLsOptions configures a GET /crawl/{uuid}/urls request.
type CrawlURLsOptions struct {
	// Status filters by URL status. Empty = server default ("visited").
	// Valid values: "visited", "pending", "failed", "skipped".
	Status string
	// Page is the 1-based page number. Zero = 1.
	Page int
	// PerPage is the page size. Zero = 100.
	PerPage int
}

// CrawlURLs lists crawled URLs for a job, streaming the response as text.
//
// `GET /crawl/{uuid}/urls` — the server returns `text/plain` with one record
// per line. JSON is intentionally NOT used: this endpoint is expected to
// scale to millions of records per job where JSON would be too expensive.
//
// Wire format:
//   - `visited` / `pending` → one URL per line
//   - `failed` / `skipped` → `url,reason` per line
//
// Pagination: the wire protocol carries no global total count. Request
// further pages by incrementing Page until an empty response.
func (c *Client) CrawlURLs(uuid string, opts *CrawlURLsOptions) (*CrawlerURLs, error) {
	if uuid == "" {
		return nil, fmt.Errorf("%w: uuid must be a non-empty string", ErrCrawlerConfig)
	}

	if opts == nil {
		opts = &CrawlURLsOptions{}
	}
	page := opts.Page
	if page == 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage == 0 {
		perPage = 100
	}
	statusHint := opts.Status
	if statusHint == "" {
		statusHint = "visited"
	}

	endpointURL, _ := url.Parse(c.host + "/crawl/" + url.PathEscape(uuid) + "/urls")
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("page", strconv.Itoa(page))
	q.Set("per_page", strconv.Itoa(perPage))
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	endpointURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	// text/plain is canonical; also accept JSON because error envelopes come
	// back as JSON regardless of the success response type.
	req.Header.Set("Accept", "text/plain, application/json")

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
	}

	// Error envelopes come back as JSON even for text endpoints. Detect by
	// content-type before consuming as text.
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return nil, fmt.Errorf(
			"%w: GET /crawl/%s/urls returned JSON on a 200 response (expected text/plain): %s",
			ErrUnexpectedResponseFormat, uuid, truncate(string(bodyBytes), 500),
		)
	}

	return parseCrawlerURLs(string(bodyBytes), statusHint, page, perPage), nil
}

// CrawlContentsOptions configures a GET /crawl/{uuid}/contents request.
type CrawlContentsOptions struct {
	// Format selects the content format. Required.
	Format CrawlerContentFormat
	// URL targets a single crawled URL. Required when Plain=true.
	URL string
	// Plain=true returns the raw content for a single URL (requires URL) as
	// a string with Content-Type matching the format. Default false returns
	// the JSON envelope wrapped in CrawlerContents.
	Plain bool
	// Limit caps the number of URLs in bulk JSON mode (max 50, default 10).
	Limit int
	// Offset skips N URLs in bulk JSON mode.
	Offset int
}

// CrawlContents fetches crawled content for a job.
//
// Two modes (selected by Plain):
//
//  1. Bulk JSON (Plain=false, default): returns *CrawlerContents with a
//     {contents: {url: {format: content}}, links} envelope.
//  2. Plain single-URL (Plain=true): returns a string containing the raw
//     content for the given URL/format. Requires URL to be set.
//
// Returns either (*CrawlerContents, nil) or ("plain string", nil). Callers
// should type-assert on the interface{} return. Go's lack of sum types makes
// this awkward — use CrawlContentsJSON or CrawlContentsPlain directly when
// you know which mode you want.
//
// Example:
//
//	contents, err := client.CrawlContentsJSON(uuid, scrapfly.CrawlerFormatMarkdown, nil)
//	md, err := client.CrawlContentsPlain(uuid, "https://example.com/page", scrapfly.CrawlerFormatMarkdown)
func (c *Client) CrawlContentsJSON(uuid string, format CrawlerContentFormat, opts *CrawlContentsOptions) (*CrawlerContents, error) {
	if opts == nil {
		opts = &CrawlContentsOptions{}
	}
	opts.Plain = false
	opts.Format = format
	body, ct, err := c.crawlContentsRaw(uuid, opts)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(ct, "application/json") {
		return nil, fmt.Errorf(
			"%w: expected JSON from CrawlContentsJSON, got Content-Type=%q",
			ErrUnexpectedResponseFormat, ct,
		)
	}
	return parseCrawlerContents(body)
}

// CrawlContentsPlain fetches the raw content for a single crawled URL in
// "plain" mode. Returns the body as a string (no JSON wrapping).
//
// The Accept header is set to `*/*` since the server picks the content-type
// based on the format (text/markdown, text/html, text/plain, application/json).
func (c *Client) CrawlContentsPlain(uuid, targetURL string, format CrawlerContentFormat) (string, error) {
	if targetURL == "" {
		return "", fmt.Errorf("%w: plain mode requires a single url argument", ErrCrawlerConfig)
	}
	body, _, err := c.crawlContentsRaw(uuid, &CrawlContentsOptions{
		Format: format,
		URL:    targetURL,
		Plain:  true,
	})
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// crawlContentsRaw performs the shared HTTP call for both JSON and plain modes.
// Returns the raw body bytes and the response content-type.
func (c *Client) crawlContentsRaw(uuid string, opts *CrawlContentsOptions) ([]byte, string, error) {
	if uuid == "" {
		return nil, "", fmt.Errorf("%w: uuid must be a non-empty string", ErrCrawlerConfig)
	}
	if opts.Format == "" {
		return nil, "", fmt.Errorf("%w: format is required", ErrCrawlerConfig)
	}
	if !opts.Format.IsValid() {
		return nil, "", fmt.Errorf("%w: invalid format %q", ErrCrawlerConfig, opts.Format)
	}

	endpointURL, _ := url.Parse(c.host + "/crawl/" + url.PathEscape(uuid) + "/contents")
	q := url.Values{}
	q.Set("key", c.key)
	// Server query param is `formats` (plural), not `format`. The public docs
	// say `format` but the actual server only accepts `formats` — discovered
	// during the TS/Python SDK port.
	q.Set("formats", string(opts.Format))
	if opts.URL != "" {
		q.Set("url", opts.URL)
	}
	if opts.Plain {
		q.Set("plain", "true")
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}
	endpointURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	if opts.Plain {
		// Plain mode: server picks content-type based on format.
		req.Header.Set("Accept", "*/*")
	} else {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", c.handleCrawlerErrorResponse(resp, bodyBytes)
	}
	return bodyBytes, resp.Header.Get("Content-Type"), nil
}

// CrawlContentsBatch retrieves content for up to 100 URLs in a single
// round-trip. Returns a map of url → format → content.
//
// `POST /crawl/{uuid}/contents/batch` — the request body is a newline-
// separated list of URLs (text/plain). The response is a multipart/related
// (RFC 2387) document with one part per found URL.
//
// Formats may be any subset of the documented content formats. Each returned
// URL will have an inner map with only the formats the server found.
func (c *Client) CrawlContentsBatch(uuid string, urls []string, formats []CrawlerContentFormat) (map[string]map[string]string, error) {
	if uuid == "" {
		return nil, fmt.Errorf("%w: uuid must be a non-empty string", ErrCrawlerConfig)
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("%w: at least one URL is required", ErrCrawlerConfig)
	}
	if len(urls) > 100 {
		return nil, fmt.Errorf("%w: batch is limited to 100 URLs per request, got %d", ErrCrawlerConfig, len(urls))
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("%w: at least one format is required", ErrCrawlerConfig)
	}
	for _, f := range formats {
		if !f.IsValid() {
			return nil, fmt.Errorf("%w: invalid format %q", ErrCrawlerConfig, f)
		}
	}

	endpointURL, _ := url.Parse(c.host + "/crawl/" + url.PathEscape(uuid) + "/contents/batch")
	q := url.Values{}
	q.Set("key", c.key)
	formatStrs := make([]string, len(formats))
	for i, f := range formats {
		formatStrs[i] = string(f)
	}
	q.Set("formats", strings.Join(formatStrs, ","))
	endpointURL.RawQuery = q.Encode()

	body := []byte(strings.Join(urls, "\n"))
	req, err := http.NewRequest("POST", endpointURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "multipart/related, application/json")

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		// Unexpected: server returned JSON on success for a multipart endpoint.
		return nil, fmt.Errorf(
			"%w: CrawlContentsBatch expected multipart/related, got JSON: %s",
			ErrUnexpectedResponseFormat, truncate(string(bodyBytes), 500),
		)
	}

	return parseMultipartRelated(string(bodyBytes), contentType, formatStrs)
}

// CrawlCancel cancels a running crawler job.
//
// `POST /crawl/{uuid}/cancel` — proxied through to the scrape-engine's
// internal cancel endpoint after the API verifies job ownership. Calling
// cancel on a job that has already finished (DONE/CANCELLED) is a no-op.
//
// Returns nil on success. Wraps any `ERR::CRAWLER::*` response with
// ErrCrawlerFailed so callers can switch on errors.Is().
func (c *Client) CrawlCancel(uuid string) error {
	if uuid == "" {
		return fmt.Errorf("%w: uuid must be a non-empty string", ErrCrawlerConfig)
	}

	endpointURL, _ := url.Parse(c.host + "/crawl/" + url.PathEscape(uuid) + "/cancel")
	q := url.Values{}
	q.Set("key", c.key)
	endpointURL.RawQuery = q.Encode()

	req, err := http.NewRequest("POST", endpointURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return c.handleCrawlerErrorResponse(resp, bodyBytes)
	}
	return nil
}

// CrawlArtifact downloads a crawler job's WARC or HAR artifact.
//
// `GET /crawl/{uuid}/artifact?type=warc|har` — returns raw bytes wrapped in
// a CrawlerArtifact. The SDK does NOT bundle WARC/HAR parsers; use a library
// like nlnwa/gowarc to walk the records.
func (c *Client) CrawlArtifact(uuid string, artifactType CrawlerArtifactType) (*CrawlerArtifact, error) {
	if uuid == "" {
		return nil, fmt.Errorf("%w: uuid must be a non-empty string", ErrCrawlerConfig)
	}
	if artifactType != ArtifactTypeWARC && artifactType != ArtifactTypeHAR {
		return nil, fmt.Errorf("%w: artifact type must be 'warc' or 'har', got %q", ErrCrawlerConfig, artifactType)
	}

	endpointURL, _ := url.Parse(c.host + "/crawl/" + url.PathEscape(uuid) + "/artifact")
	q := url.Values{}
	q.Set("key", c.key)
	q.Set("type", string(artifactType))
	endpointURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	// Accept both the binary artifact types and JSON for error envelopes.
	if artifactType == ArtifactTypeHAR {
		req.Header.Set("Accept", "application/json, application/octet-stream")
	} else {
		req.Header.Set("Accept", "application/gzip, application/octet-stream, application/json")
	}

	resp, err := fetchWithRetry(c.httpClient, req, defaultRetries, defaultDelay)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
	}

	// For HAR, the body IS JSON — but we still need to detect error envelopes.
	if artifactType == ArtifactTypeHAR && strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		var raw map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &raw); err == nil {
			if _, isErr := raw["error_id"]; isErr {
				return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
			}
			if httpCode, ok := raw["http_code"]; ok && httpCode != nil {
				return nil, c.handleCrawlerErrorResponse(resp, bodyBytes)
			}
		}
	}

	return &CrawlerArtifact{Type: artifactType, Data: bodyBytes}, nil
}

// ==============================================================================
// Helpers
// ==============================================================================

// handleCrawlerErrorResponse converts a non-2xx crawler API response into a
// properly typed Go error. `ERR::CRAWLER::*` codes get wrapped with
// ErrCrawlerFailed so callers can switch on errors.Is().
func (c *Client) handleCrawlerErrorResponse(resp *http.Response, body []byte) error {
	var envelope errorResponse
	_ = json.Unmarshal(body, &envelope)

	msg := envelope.Message
	if msg == "" {
		msg = fmt.Sprintf("crawler API returned status %d", resp.StatusCode)
	}

	apiErr := &APIError{
		Message:        msg,
		Code:           envelope.Code,
		HTTPStatusCode: resp.StatusCode,
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		apiErr.Hint = "Provide a valid API key via ?key=... or Bearer token."
	case http.StatusTooManyRequests:
		apiErr.Hint = "Back off and retry after the indicated delay, or reduce concurrency."
	}

	// Map crawler-resource errors to ErrCrawlerFailed sentinel.
	if strings.Contains(envelope.Code, "::CRAWLER::") {
		return fmt.Errorf("%w: %s", ErrCrawlerFailed, apiErr)
	}
	return apiErr
}

// truncate returns the first n characters of s, appending "..." if cut.
// Used for short error message previews.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// parseMultipartRelated parses an RFC 2387 multipart/related response body
// into a map of `content-location` → `inferred-format` → body text.
//
// This is a deliberately minimal parser: it walks the body boundary-by-
// boundary, extracts `Content-Location` and `Content-Type` from each part's
// headers, and infers the format from the content-type. It avoids pulling in
// the stdlib `mime/multipart` package because that API is stream-based and
// awkward to use for small, in-memory multipart payloads.
func parseMultipartRelated(body, contentType string, formats []string) (map[string]map[string]string, error) {
	// Extract the boundary from the Content-Type header.
	// Format: `multipart/related; boundary=abc123; type="application/json"`
	boundary := ""
	for _, part := range strings.Split(contentType, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "boundary=") {
			boundary = strings.TrimPrefix(part, "boundary=")
			boundary = strings.Trim(boundary, "\"")
			break
		}
	}
	if boundary == "" {
		return nil, fmt.Errorf(
			"%w: CrawlContentsBatch response has no multipart boundary in content-type %q",
			ErrUnexpectedResponseFormat, contentType,
		)
	}

	delimiter := "--" + boundary
	result := make(map[string]map[string]string)

	// Split on the delimiter; first segment is the preamble (ignored),
	// last is the closing `--` (also ignored).
	segments := strings.Split(body, delimiter)
	for i := 1; i < len(segments); i++ {
		segment := segments[i]
		// Strip leading CRLF after the boundary.
		segment = strings.TrimPrefix(segment, "\r\n")
		segment = strings.TrimPrefix(segment, "\n")
		// The closing boundary is `--{boundary}--`; segment starts with `--`.
		if strings.HasPrefix(segment, "--") {
			break
		}
		// Trim trailing CRLF before the next boundary.
		segment = strings.TrimSuffix(segment, "\r\n")
		segment = strings.TrimSuffix(segment, "\n")

		// Header/body split on the first blank line.
		headerEnd := strings.Index(segment, "\r\n\r\n")
		sepLen := 4
		if headerEnd == -1 {
			headerEnd = strings.Index(segment, "\n\n")
			sepLen = 2
		}
		if headerEnd == -1 {
			continue
		}
		headersRaw := segment[:headerEnd]
		partBody := segment[headerEnd+sepLen:]

		var partURL, partFormat string
		for _, line := range strings.Split(headersRaw, "\n") {
			line = strings.TrimRight(line, "\r")
			colon := strings.Index(line, ":")
			if colon == -1 {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(line[:colon]))
			value := strings.TrimSpace(line[colon+1:])
			switch name {
			case "content-location":
				partURL = value
			case "content-type":
				partFormat = inferFormatFromContentType(value)
			}
		}
		if partURL == "" {
			continue
		}
		if partFormat == "" {
			// Fall back to the first requested format if the part has no content-type.
			if len(formats) > 0 {
				partFormat = formats[0]
			} else {
				partFormat = "html"
			}
		}
		if result[partURL] == nil {
			result[partURL] = make(map[string]string)
		}
		result[partURL][partFormat] = partBody
	}
	return result, nil
}

// inferFormatFromContentType maps a MIME type back to a crawler content format.
func inferFormatFromContentType(ct string) string {
	lc := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch lc {
	case "text/html":
		return "html"
	case "text/markdown":
		return "markdown"
	case "text/plain":
		return "text"
	case "application/json":
		return "json"
	}
	return ""
}
