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
	key        string
	host       string
	httpClient *http.Client
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
	params, err := config.toAPIParams()
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

// ConcurrentScrape performs multiple scraping requests concurrently with controlled concurrency.
// This is useful for scraping multiple pages efficiently while respecting rate limits.
//
// Parameters:
//   - configs: A slice of ScrapeConfig objects to scrape
//   - concurrencyLimit: Maximum number of concurrent requests. If <= 0, uses account's concurrent limit
//
// Returns a channel that emits results as they complete. Each result contains either
// a successful ScrapeResult or an error.
//
// Example:
//
//	configs := []*scrapfly.ScrapeConfig{
//	    {URL: "https://example.com/page1"},
//	    {URL: "https://example.com/page2"},
//	    {URL: "https://example.com/page3"},
//	}
//	resultsChan := client.ConcurrentScrape(configs, 3)
//	for result := range resultsChan {
//	    if result.error != nil {
//	        log.Printf("Error: %v", result.error)
//	        continue
//	    }
//	    fmt.Println(result.ScrapeResult.Result.Content)
//	}
func (c *Client) ConcurrentScrape(configs []*ScrapeConfig, concurrencyLimit int) <-chan struct {
	*ScrapeResult
	error
} {
	resultsChan := make(chan struct {
		*ScrapeResult
		error
	}, len(configs))

	var wg sync.WaitGroup

	if concurrencyLimit <= 0 {
		account, err := c.Account()
		if err != nil {
			resultsChan <- struct {
				*ScrapeResult
				error
			}{nil, fmt.Errorf("failed to get account for concurrency limit: %w", err)}
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
				resultsChan <- struct {
					*ScrapeResult
					error
				}{result, err}
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
