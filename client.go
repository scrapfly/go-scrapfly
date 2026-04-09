package scrapfly

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultHost    = "https://api.scrapfly.io"
	defaultRetries = 3
	defaultDelay   = 1 * time.Second
	sdkUserAgent   = "Scrapfly-Go-SDK"
)

// Client is the main client for interacting with the Scrapfly API.
// It handles authentication, request execution, and response parsing.
type Client struct {
	key              string
	host             string
	cloudBrowserHost string
	httpClient       *http.Client
}

// SetCloudBrowserHost overrides the default Cloud Browser host
// (wss://browser.scrapfly.io). Useful for self-hosted or development
// environments. When unset, CloudBrowser methods fall back to the
// public production host.
func (c *Client) SetCloudBrowserHost(host string) {
	c.cloudBrowserHost = host
}

// SetHTTPClient replaces the underlying *http.Client used for all API calls.
// This lets callers install a custom transport (e.g. for request logging,
// tracing, tests, or a shared connection pool).
//
// Passing nil is a no-op.
//
// Example — install a RoundTripper that logs every request:
//
//	client, _ := scrapfly.New("YOUR_API_KEY")
//	client.SetHTTPClient(&http.Client{
//	    Transport: &loggingTransport{base: http.DefaultTransport},
//	    Timeout:   150 * time.Second,
//	})
func (c *Client) SetHTTPClient(httpClient *http.Client) {
	if httpClient == nil {
		return
	}
	c.httpClient = httpClient
}

// HTTPClient returns the *http.Client used by this Scrapfly client.
// Useful if callers want to wrap the existing transport instead of replacing it.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// New creates a new Scrapfly client with the provided API key.
// The API key can be obtained from https://scrapfly.io/dashboard.
//
// Example:
//
//	client, err := scrapfly.New("YOUR_API_KEY")
//	if err != nil {
//	    log.Fatal(err)
//	}
func New(key string) (*Client, error) {
	if key == "" {
		return nil, ErrBadAPIKey
	}
	return &Client{
		key:        key,
		host:       defaultHost,
		httpClient: &http.Client{Timeout: 150 * time.Second},
	}, nil
}

// NewWithHost creates a new Scrapfly client with a custom API host.
// This is useful for enterprise deployments or testing against a custom endpoint.
//
// Parameters:
//   - key: Your Scrapfly API key
//   - host: Custom API host URL (e.g., "https://custom-api.example.com")
//   - verifySSL: Whether to verify SSL certificates (set to false only for testing)
//
// Example:
//
//	client, err := scrapfly.NewWithHost("YOUR_API_KEY", "https://custom-api.example.com", true)
func NewWithHost(key, host string, verifySSL bool) (*Client, error) {
	if key == "" {
		return nil, ErrBadAPIKey
	}
	if !verifySSL {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		key:        key,
		host:       host,
		httpClient: &http.Client{Timeout: 150 * time.Second},
	}, nil
}

// APIKey returns the currently configured API key.
func (c *Client) APIKey() string {
	return c.key
}

// SetAPIKey updates the API key for the client.
// This is useful for switching between different API keys at runtime.
func (c *Client) SetAPIKey(key string) {
	c.key = key
}

// VerifyAPIKey checks if the configured API key is valid.
// Returns a VerifyAPIKeyResult indicating whether the key is valid.
//
// Example:
//
//	result, err := client.VerifyAPIKey()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if result.Valid {
//	    fmt.Println("API key is valid")
//	}
func (c *Client) VerifyAPIKey() (*VerifyAPIKeyResult, error) {
	endpointURL, _ := url.Parse(c.host + "/account")
	params := url.Values{}
	params.Set("key", c.key)
	endpointURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		return &VerifyAPIKeyResult{Valid: true}, nil
	}
	return &VerifyAPIKeyResult{Valid: false}, nil
}

// Scrape performs a web scraping request using the provided configuration.
// This is the main method for scraping web pages with Scrapfly.
//
// The method supports various features including:
//   - JavaScript rendering (RenderJS)
//   - Proxy rotation and geo-targeting
//   - Anti-bot protection (ASP)
//   - Custom headers and cookies
//   - Screenshot capture
//   - Data extraction
//
// Example:
//
//	config := &scrapfly.ScrapeConfig{
//	    URL:      "https://example.com",
//	    RenderJS: true,
//	    Country:  "us",
//	    ASP:      true,
//	}
//	result, err := client.Scrape(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(result.Result.Content)
func (c *Client) Scrape(config *ScrapeConfig) (*ScrapeResult, error) {
	DefaultLogger.Debug("scraping", "url", config.URL)

	if err := config.processBody(); err != nil {
		return nil, err
	}
	params, err := config.toAPIParamsWithValidation()
	if err != nil {
		return nil, err
	}
	params.Set("key", c.key)

	endpointURL, _ := url.Parse(c.host + "/scrape")
	endpointURL.RawQuery = params.Encode()

	method := "GET"
	if config.Method != "" {
		method = strings.ToUpper(config.Method.String())
	}

	req, err := http.NewRequest(method, endpointURL.String(), strings.NewReader(config.Body))
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(config.Body)), nil
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
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
		return nil, c.handleAPIErrorResponse(resp, bodyBytes)
	}

	var result ScrapeResult
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scrape result: %w", err)
	}
	if result.Result.Success && result.Result.Status == "DONE" {
		DefaultLogger.Debug("scrape log url:", result.Result.LogURL)

		// handle large objects (clob/blob formats)
		contentFormat := result.Result.Format
		if contentFormat == "clob" || contentFormat == "blob" {
			newContent, newFormat, err := c.handleLargeObjects(result.Result.Content, contentFormat)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch large object: %w", err)
			}
			result.Result.Content = newContent
			result.Result.Format = newFormat
		}
		/////////////////////////////////////////

		// Add back apiKey to screenshots URLs
		for name, screenshot := range result.Result.Screenshots {
			newScreenshot := Screenshot{
				URL:         screenshot.URL + "?key=" + c.key,
				Extension:   screenshot.Extension,
				Format:      screenshot.Format,
				Size:        screenshot.Size,
				CSSSelector: screenshot.CSSSelector,
				Name:        name,
			}
			result.Result.Screenshots[name] = newScreenshot
		}

		// Add back apiKey to attachments URLs
		for i, attachment := range result.Result.BrowserData.Attachments {
			newAttachment := Attachment{
				Content:           attachment.Content + "?key=" + c.key,
				ContentType:       attachment.ContentType,
				Filename:          attachment.Filename,
				ID:                attachment.ID,
				Size:              attachment.Size,
				State:             attachment.State,
				SuggestedFilename: attachment.SuggestedFilename,
				URL:               attachment.URL,
			}
			result.Result.BrowserData.Attachments[i] = newAttachment
		}
		/////////////////////////////////////////

		return &result, nil
	}
	return nil, c.createErrorFromResult(&result)
}

// handleLargeObjects fetches content for large objects (clob/blob formats) using the internal API key.
func (c *Client) handleLargeObjects(contentURL string, format string) (string, string, error) {
	parsedURL, err := url.Parse(contentURL)
	if err != nil {
		DefaultLogger.Error("failed to parse content URL:", err)
		return "", "", err
	}
	params := parsedURL.Query()
	params.Set("key", c.APIKey())
	parsedURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		DefaultLogger.Error("failed to fetch large object:", err)
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("failed to fetch large object: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	switch format {
	case "clob":
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", fmt.Errorf("failed to read clob response: %w", err)
		}
		return string(bodyBytes), "text", nil
	case "blob":
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", "", fmt.Errorf("failed to read blob response: %w", err)
		}
		return string(bodyBytes), "binary", nil
	default:
		return "", "", fmt.Errorf("unsupported format: %s", format)
	}
}

// ConcurrentScrapeResult is one entry in the channel returned by ConcurrentScrape.
// Exactly one of Result and Error is non-nil per emission.
//
// This was previously an anonymous struct with embedded fields, which prevented
// callers outside package scrapfly from accessing the error (Go's universe-scope
// `error` type produces an unexported promoted field in anonymous structs).
// Named exported fields make the result usable from any caller.
type ConcurrentScrapeResult struct {
	// Result is the successful scrape, or nil when Error is set.
	Result *ScrapeResult
	// Error is the failure, or nil when Result is set.
	Error error
}

// ConcurrentScrape performs multiple scraping requests concurrently with controlled concurrency.
// This is useful for scraping multiple pages efficiently while respecting rate limits.
//
// Parameters:
//   - configs: A slice of ScrapeConfig objects to scrape
//   - concurrencyLimit: Maximum number of concurrent requests. If <= 0, uses account's concurrent limit
//
// Returns a channel that emits ConcurrentScrapeResult values as scrapes complete.
// Each entry has either Result (success) or Error (failure) set.
//
// Example:
//
//	configs := []*scrapfly.ScrapeConfig{
//	    {URL: "https://example.com/page1"},
//	    {URL: "https://example.com/page2"},
//	    {URL: "https://example.com/page3"},
//	}
//	for item := range client.ConcurrentScrape(configs, 3) {
//
// ScrapeProxified sends a scrape request with proxified_response=true and returns
// the raw upstream *http.Response. The caller owns resp.Body (must Close() it).
//
// Unlike Scrape(), no JSON parsing occurs — the response body is the target
// page's raw content, the status code is the upstream status, and Scrapfly
// metadata is available on X-Scrapfly-* response headers (Api-Cost,
// Content-Format, Log, etc).
//
// Use this when you want Scrapfly to act like an HTTP proxy and your code
// already knows how to handle raw HTTP responses.
func (c *Client) ScrapeProxified(config *ScrapeConfig) (*http.Response, error) {
	config.ProxifiedResponse = true

	if err := config.processBody(); err != nil {
		return nil, err
	}
	params, err := config.toAPIParamsWithValidation()
	if err != nil {
		return nil, err
	}
	params.Set("key", c.key)

	endpointURL, _ := url.Parse(c.host + "/scrape")
	endpointURL.RawQuery = params.Encode()

	method := "GET"
	if config.Method != "" {
		method = strings.ToUpper(config.Method.String())
	}

	req, err := http.NewRequest(method, endpointURL.String(), strings.NewReader(config.Body))
	if err != nil {
		return nil, err
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	// Error restoration: if X-Scrapfly-Reject-Code is present, the
	// scrape failed. Close the body and return a typed error so callers
	// get the same interface as non-proxified mode.
	if rejectCode := resp.Header.Get("X-Scrapfly-Reject-Code"); rejectCode != "" {
		defer resp.Body.Close()
		rejectDesc := resp.Header.Get("X-Scrapfly-Reject-Description")
		retryable := resp.Header.Get("X-Scrapfly-Reject-Retryable") == "true"
		retryAfterMs := 0
		if retryable {
			if ra, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
				retryAfterMs = ra * 1000 // Retry-After is in seconds
			}
		}
		return nil, &APIError{
			Message:        fmt.Sprintf("Proxified scrape error: %s — %s", rejectCode, rejectDesc),
			Code:           rejectCode,
			HTTPStatusCode: resp.StatusCode,
			Retryable:      retryable,
			RetryAfterMs:   retryAfterMs,
		}
	}
	// Caller owns the body — do NOT defer resp.Body.Close() here.
	return resp, nil
}

//	    if item.Error != nil {
//	        log.Printf("Error: %v", item.Error)
//	        continue
//	    }
//	    fmt.Println(item.Result.Result.Content)
//	}
func (c *Client) ConcurrentScrape(configs []*ScrapeConfig, concurrencyLimit int) <-chan ConcurrentScrapeResult {
	resultsChan := make(chan ConcurrentScrapeResult, len(configs))

	var wg sync.WaitGroup

	if concurrencyLimit <= 0 {
		account, err := c.Account()
		if err != nil {
			resultsChan <- ConcurrentScrapeResult{
				Result: nil,
				Error:  fmt.Errorf("failed to get account for concurrency limit: %w", err),
			}
			close(resultsChan)
			return resultsChan
		}
		concurrencyLimit = account.Subscription.Usage.Scrape.ConcurrentLimit
		DefaultLogger.Info("concurrency not provided - setting it to", concurrencyLimit, "from account info")
	}

	jobs := make(chan *ScrapeConfig, len(configs))
	for i := 0; i < concurrencyLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for config := range jobs {
				result, err := c.Scrape(config)
				resultsChan <- ConcurrentScrapeResult{Result: result, Error: err}
			}
		}()
	}

	for _, config := range configs {
		jobs <- config
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	return resultsChan
}

// Screenshot captures a screenshot of a web page using the provided configuration.
//
// Supports various features including:
//   - Multiple image formats (JPG, PNG, WEBP, GIF)
//   - Full page or element-specific capture
//   - Custom resolution
//   - Dark mode
//   - Banner blocking
//
// Example:
//
//	config := &scrapfly.ScreenshotConfig{
//	    URL:        "https://example.com",
//	    Format:     scrapfly.FormatPNG,
//	    Capture:    "fullpage",
//	    Resolution: "1920x1080",
//	}
//	result, err := client.Screenshot(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// result.Image contains the screenshot bytes
func (c *Client) Screenshot(config *ScreenshotConfig) (*ScreenshotResult, error) {
	params, err := config.toAPIParams()
	if err != nil {
		return nil, err
	}
	params.Set("key", c.key)

	endpointURL, _ := url.Parse(c.host + "/screenshot")
	endpointURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)

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
		return nil, c.handleAPIErrorResponse(resp, bodyBytes)
	}

	return newScreenshotResult(resp, bodyBytes)
}

// Extract performs AI-powered structured data extraction from HTML content.
//
// This method uses Scrapfly's AI extraction capabilities to parse HTML and
// extract structured data based on templates or prompts.
//
// Example:
//
//	config := &scrapfly.ExtractionConfig{
//	    Body:               []byte("<html>...</html>"),
//	    ContentType:        "text/html",
//	    ExtractionTemplate: "product",
//	}
//	result, err := client.Extract(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Extracted data: %+v\n", result.Data)
func (c *Client) Extract(config *ExtractionConfig) (*ExtractionResult, error) {
	params, err := config.toAPIParams()
	if err != nil {
		return nil, err
	}
	params.Set("key", c.key)

	endpointURL, _ := url.Parse(c.host + "/extraction")
	endpointURL.RawQuery = params.Encode()

	req, err := http.NewRequest("POST", endpointURL.String(), bytes.NewReader(config.Body))
	if err != nil {
		return nil, err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(config.Body)), nil
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Content-Type", config.ContentType)
	req.Header.Set("Accept", "application/json")
	if config.DocumentCompressionFormat != "" {
		req.Header.Set("Content-Encoding", string(config.DocumentCompressionFormat))
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
		return nil, c.handleAPIErrorResponse(resp, bodyBytes)
	}

	var result ExtractionResult
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extraction result: %w", err)
	}
	return &result, nil
}

// Account retrieves information about the current Scrapfly account.
//
// Returns account details including:
//   - Subscription plan and limits
//   - API usage statistics
//   - Billing information
//   - Concurrency limits
//
// Example:
//
//	account, err := client.Account()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Plan: %s\n", account.Subscription.PlanName)
//	fmt.Printf("Remaining requests: %d\n", account.Subscription.Usage.Scrape.Remaining)
func (c *Client) Account() (*AccountData, error) {
	endpointURL, _ := url.Parse(c.host + "/account")
	params := url.Values{}
	params.Set("key", c.key)
	endpointURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", endpointURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIErrorResponse(resp, bodyBytes)
	}

	var data AccountData
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account data: %w", err)
	}
	return &data, nil
}

func (c *Client) handleAPIErrorResponse(resp *http.Response, body []byte) error {
	statusCode := resp.StatusCode

	var result ScrapeResult
	if err := json.Unmarshal(body, &result); err == nil {
		if result.Result.Error != nil {
			apiErr := &APIError{
				APIResponse:    &result,
				HTTPStatusCode: resp.StatusCode,
			}
			if result.Result.Error != nil {
				apiErr.Message = result.Result.Error.Message
				apiErr.Code = result.Result.Error.Code
				apiErr.DocumentationURL = result.Result.Error.DocURL
			} else {
				apiErr.Message = "scrape failed with status: " + result.Result.Status
				apiErr.Code = result.Result.Status
			}
			return apiErr
		}
	}

	var errResp errorResponse
	_ = json.Unmarshal(body, &errResp)
	msg := errResp.Message
	if msg == "" {
		msg = fmt.Sprintf("API returned status %d", statusCode)
	}

	apiErr := &APIError{
		Message:        msg,
		HTTPStatusCode: statusCode,
		Code:           errResp.Code,
	}

	// Retry-After parsing (seconds or HTTP-date)
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs >= 0 {
			apiErr.RetryAfterMs = secs * 1000
		} else if t, err := http.ParseTime(ra); err == nil {
			ms := int(time.Until(t).Milliseconds())
			if ms < 0 {
				ms = 0
			}
			apiErr.RetryAfterMs = ms
		}
	}

	switch statusCode {
	case http.StatusUnauthorized:
		apiErr.Hint = "Provide a valid API key via ?key=... or Bearer token (cloud mode)."
	case http.StatusTooManyRequests:
		apiErr.Hint = "Back off and retry after the indicated delay, or reduce concurrency/scope."
	case http.StatusUnprocessableEntity:
		if strings.Contains(string(body), "SCREENSHOT") {
			apiErr.Hint = "Check screenshot parameters (format/capture/resolution) and upstream site readiness."
		}
		if strings.Contains(string(body), "EXTRACTION") {
			apiErr.Hint = "Check content_type, body encoding, and template/prompt validity."
		}
	}

	return apiErr
}

func (c *Client) createErrorFromResult(result *ScrapeResult) error {
	apiErr := &APIError{
		APIResponse:    result,
		HTTPStatusCode: result.Result.StatusCode,
	}
	if result.Result.Error != nil {
		apiErr.Message = result.Result.Error.Message
		apiErr.Code = result.Result.Error.Code
		apiErr.DocumentationURL = result.Result.Error.DocURL
	} else {
		apiErr.Message = "scrape failed with status: " + result.Result.Status
		apiErr.Code = result.Result.Status
	}

	if !result.Result.Success {
		if result.Result.StatusCode >= 400 && result.Result.StatusCode < 500 {
			return fmt.Errorf("%w: %s", ErrUpstreamClient, apiErr)
		}
		if result.Result.StatusCode >= 500 {
			return fmt.Errorf("%w: %s", ErrUpstreamServer, apiErr)
		}
	}

	if parts := strings.Split(result.Result.Status, "::"); len(parts) > 1 {
		resource := parts[1]
		switch resource {
		case "SCRAPE":
			return fmt.Errorf("%w: %s", ErrScrapeFailed, apiErr)
		case "PROXY":
			return fmt.Errorf("%w: %s", ErrProxyFailed, apiErr)
		case "ASP":
			return fmt.Errorf("%w: %s", ErrASPBypassFailed, apiErr)
		case "SCHEDULE":
			return fmt.Errorf("%w: %s", ErrScheduleFailed, apiErr)
		case "WEBHOOK":
			return fmt.Errorf("%w: %s", ErrWebhookFailed, apiErr)
		case "SESSION":
			return fmt.Errorf("%w: %s", ErrSessionFailed, apiErr)
		}
	}
	return fmt.Errorf("%w: %s", ErrUnhandledAPIResponse, apiErr)
}
