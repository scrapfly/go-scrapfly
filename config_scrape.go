package scrapfly

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	js_scenario "github.com/scrapfly/go-scrapfly/scenario"
)

// ScrapeConfig configures a web scraping request to the Scrapfly API.
//
// This struct contains all available options for customizing scraping behavior,
// including proxy settings, JavaScript rendering, data extraction, and more.
//
// Example:
//
//	config := &scrapfly.ScrapeConfig{
//	    URL:      "https://example.com",
//	    RenderJS: true,
//	    Country:  "us",
//	    ASP:      true,
//	    Cache:    true,
//	    Format:   scrapfly.FormatMarkdown,
//	}
type ScrapeConfig struct {
	// URL is the target URL to scrape (required).
	URL string
	// Method is the HTTP method to use (GET, POST, PUT, PATCH). Defaults to GET.
	Method HttpMethod
	// Body is the raw request body for POST/PUT/PATCH requests.
	Body string
	// Data is a map that will be encoded as request body based on Content-Type.
	// Cannot be used together with Body.
	Data map[string]interface{}
	// Headers are custom HTTP headers to send with the request.
	Headers map[string]string
	// Cookies are cookies to include in the request.
	Cookies map[string]string
	// Country specifies the proxy country code (e.g., "us", "uk", "de").
	Country string
	// ProxyPool specifies which proxy pool to use.
	ProxyPool ProxyPool
	// RenderJS enables JavaScript rendering using a headless browser.
	RenderJS bool
	// ASP enables Anti-Scraping Protection bypass.
	ASP bool
	// Cache enables response caching.
	Cache bool
	// CacheTTL sets the cache time-to-live in seconds.
	CacheTTL int
	// CacheClear forces cache refresh for this request.
	CacheClear bool
	// Timeout sets the maximum time in milliseconds to wait for the request.
	Timeout int
	// Retry enables automatic retries on failure (enabled by default).
	Retry bool
	// Session maintains a persistent browser session across requests.
	Session string
	// SessionStickyProxy keeps the same proxy for all requests in a session.
	SessionStickyProxy bool
	// Tags are custom tags for organizing and filtering requests.
	Tags []string
	// Webhook is the name of a webhook to call after the request completes.
	Webhook string
	// Debug enables debug mode for viewing request details in the dashboard.
	Debug bool
	// SSL enables SSL certificate verification details capture.
	SSL bool
	// DNS enables DNS resolution details capture.
	DNS bool
	// CorrelationID is a custom ID for tracking requests across systems.
	CorrelationID string
	// Format specifies the output format for the scraped content.
	Format Format
	// FormatOptions are additional options for the content format.
	FormatOptions []FormatOption
	// ExtractionTemplate is the name of a saved extraction template.
	ExtractionTemplate string
	// ExtractionEphemeralTemplate is an inline extraction template definition.
	ExtractionEphemeralTemplate map[string]interface{}
	// ExtractionPrompt is an AI prompt for extracting structured data.
	ExtractionPrompt string
	// ExtractionModel specifies which AI model to use for extraction.
	ExtractionModel string
	// WaitForSelector waits for a CSS selector to appear before capturing (requires RenderJS).
	WaitForSelector string
	// RenderingWait is additional wait time in milliseconds after page load (requires RenderJS).
	RenderingWait int
	// AutoScroll automatically scrolls the page to load lazy content (requires RenderJS).
	AutoScroll bool
	// Screenshots is a map of screenshot names to CSS selectors (requires RenderJS).
	Screenshots map[string]string
	// ScreenshotFlags are options for screenshot capture.
	ScreenshotFlags []ScreenshotFlag
	// JS is custom JavaScript code to execute in the browser (requires RenderJS).
	JS string
	// JSScenario is a sequence of browser actions to perform (requires RenderJS).
	JSScenario []js_scenario.JSScenarioStep
	// OS spoofs the operating system in the User-Agent.
	OS string
	// Lang sets the Accept-Language header values.
	Lang []string
}

// toAPIParams converts the ScrapeConfig into URL parameters for the Scrapfly API.
// This is an internal method used by the Client to prepare API requests.
func (c *ScrapeConfig) toAPIParams() (url.Values, error) {
	params := url.Values{}

	if c.URL == "" {
		return nil, fmt.Errorf("%w: URL is required", ErrScrapeConfig)
	}
	params.Set("url", c.URL)

	if c.Country != "" {
		params.Set("country", c.Country)
	}
	if c.ProxyPool != "" {
		params.Set("proxy_pool", string(c.ProxyPool))
	}

	if c.RenderJS {
		params.Set("render_js", "true")
		if c.WaitForSelector != "" {
			params.Set("wait_for_selector", c.WaitForSelector)
		}
		if c.RenderingWait > 0 {
			params.Set("rendering_wait", fmt.Sprint(c.RenderingWait))
		}
		if c.AutoScroll {
			params.Set("auto_scroll", "true")
		}
		if c.JS != "" {
			params.Set("js", urlSafeB64Encode(c.JS))
		}
		if len(c.JSScenario) > 0 {
			scenarioJSON, err := json.Marshal(c.JSScenario)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal js_scenario: %w", err)
			}
			params.Set("js_scenario", urlSafeB64Encode(string(scenarioJSON)))
		}
		if len(c.Screenshots) > 0 {
			for name, value := range c.Screenshots {
				if value == "" {
					return nil, fmt.Errorf("%w: screenshots[%s] require either a selector or fullpage", ErrScrapeConfig, name)
				}
				params.Set(fmt.Sprintf("screenshots[%s]", name), value)
			}
		}
		if len(c.ScreenshotFlags) > 0 {
			var flags []string
			for _, flag := range c.ScreenshotFlags {
				if flag.IsValid() {
					flags = append(flags, string(flag))
				}
			}
			params.Set("screenshot_flags", strings.Join(flags, ","))
		}
	}

	if c.ASP {
		params.Set("asp", "true")
	}
	if !c.Retry {
		params.Set("retry", "false")
	}
	if c.Cache {
		params.Set("cache", "true")
		if c.CacheTTL > 0 {
			params.Set("cache_ttl", fmt.Sprint(c.CacheTTL))
		}
		if c.CacheClear {
			params.Set("cache_clear", "true")
		}
	}
	if c.Timeout > 0 {
		params.Set("timeout", fmt.Sprint(c.Timeout))
	}
	if c.Debug {
		params.Set("debug", "true")
	}
	if c.SSL {
		params.Set("ssl", "true")
	}
	if c.DNS {
		params.Set("dns", "true")
	}
	if c.CorrelationID != "" {
		params.Set("correlation_id", c.CorrelationID)
	}

	if len(c.Tags) > 0 {
		params.Set("tags", strings.Join(c.Tags, ","))
	}
	if c.Webhook != "" {
		params.Set("webhook_name", c.Webhook)
	}

	if c.Session != "" {
		params.Set("session", c.Session)
		if c.SessionStickyProxy {
			params.Set("session_sticky_proxy", "true")
		}
	}

	if c.OS != "" {
		params.Set("os", c.OS)
	}
	if len(c.Lang) > 0 {
		params.Set("lang", strings.Join(c.Lang, ","))
	}

	if c.Format != "" {
		if !c.Format.IsValid() {
			return nil, fmt.Errorf("%w: invalid format: %s", ErrScrapeConfig, c.Format)
		}
		formatVal := c.Format.String()
		if len(c.FormatOptions) > 0 {
			var opts []string
			for _, opt := range c.FormatOptions {
				if !opt.IsValid() {
					return nil, fmt.Errorf("%w: invalid format option: %s", ErrScrapeConfig, opt)
				}
				opts = append(opts, string(opt))
			}
			formatVal += ":" + strings.Join(opts, ",")
		}
		params.Set("format", formatVal)
	}

	if c.ExtractionTemplate != "" && c.ExtractionEphemeralTemplate != nil {
		return nil, fmt.Errorf("%w: cannot use both extraction_template and extraction_ephemeral_template", ErrScrapeConfig)
	}
	if c.ExtractionTemplate != "" {
		params.Set("extraction_template", c.ExtractionTemplate)
	}
	if c.ExtractionEphemeralTemplate != nil {
		templateJSON, err := json.Marshal(c.ExtractionEphemeralTemplate)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal extraction_ephemeral_template: %w", err)
		}
		params.Set("extraction_template", "ephemeral:"+urlSafeB64Encode(string(templateJSON)))
	}
	if c.ExtractionPrompt != "" {
		params.Set("extraction_prompt", c.ExtractionPrompt)
	}
	if c.ExtractionModel != "" {
		params.Set("extraction_model", c.ExtractionModel)
	}

	for key, value := range c.Headers {
		if key == "" || value == "" {
			return nil, fmt.Errorf("%w: headers key and value cannot be empty, found key: %s, value: %s", ErrScrapeConfig, key, value)
		}
		params.Set(fmt.Sprintf("headers[%s]", strings.ToLower(key)), value)
	}

	if len(c.Cookies) > 0 {
		var cookieParts []string
		for name, value := range c.Cookies {
			if name == "" || value == "" {
				return nil, fmt.Errorf("%w: cookies name and value cannot be empty, found name: %s, value: %s", ErrScrapeConfig, name, value)
			}
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
		}
		cookieHeader := strings.Join(cookieParts, "; ")

		existingCookie := ""
		for k, v := range c.Headers {
			if strings.ToLower(k) == "cookie" {
				existingCookie = v
			}
		}
		if existingCookie != "" {
			params.Set("headers[cookie]", existingCookie+"; "+cookieHeader)
		} else {
			params.Set("headers[cookie]", cookieHeader)
		}
	}

	return params, nil
}

// processBody handles the Data and Body fields for POST/PUT/PATCH requests.
// It converts the Data map to the appropriate body format based on Content-Type.
// This is an internal method used during request preparation.
func (c *ScrapeConfig) processBody() error {
	method := strings.ToUpper(c.Method.String())
	if method != "POST" && method != "PUT" && method != "PATCH" {
		return nil
	}

	if c.Body != "" && c.Data != nil {
		return fmt.Errorf("%w: cannot set both Body and Data", ErrScrapeConfig)
	}

	if c.Data != nil {
		contentType, ok := c.Headers["content-type"]
		if !ok {
			contentType = "application/x-www-form-urlencoded"
			if c.Headers == nil {
				c.Headers = make(map[string]string)
			}
			c.Headers["content-type"] = contentType
		}

		switch {
		case strings.Contains(contentType, "application/json"):
			jsonData, err := json.Marshal(c.Data)
			if err != nil {
				return err
			}
			c.Body = string(jsonData)
		case strings.Contains(contentType, "application/x-www-form-urlencoded"):
			values := url.Values{}
			for k, v := range c.Data {
				values.Set(k, fmt.Sprint(v))
			}
			c.Body = values.Encode()
		default:
			return fmt.Errorf("%w: unsupported content-type for Data field: %s. Use Body field instead", ErrScrapeConfig, contentType)
		}
	}

	if c.Body != "" {
		if _, ok := c.Headers["content-type"]; !ok {
			if c.Headers == nil {
				c.Headers = make(map[string]string)
			}
			c.Headers["content-type"] = "text/plain"
		}
	}
	return nil
}
