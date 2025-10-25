package scrapfly

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type ProxyPool string

// Proxy Pools
const (
	PublicDataCenterPool  ProxyPool = "public_datacenter_pool"
	PublicResidentialPool ProxyPool = "public_residential_pool"
)

// ScreenshotFlags define options for screenshot behavior.
type ScreenshotFlag string

const (
	LoadImages       ScreenshotFlag = "load_images"
	DarkMode         ScreenshotFlag = "dark_mode"
	BlockBanners     ScreenshotFlag = "block_banners"
	PrintMediaFormat ScreenshotFlag = "print_media_format"
	HighQuality      ScreenshotFlag = "high_quality"
)

// Format defines the scraped content format.
type Format string

const (
	FormatJSON      Format = "json"
	FormatText      Format = "text"
	FormatMarkdown  Format = "markdown"
	FormatCleanHTML Format = "clean_html"
	FormatRaw       Format = "raw"
)

// FormatOption defines options for content formatting.
type FormatOption string

const (
	NoLinks     FormatOption = "no_links"
	NoImages    FormatOption = "no_images"
	OnlyContent FormatOption = "only_content"
)

type JSScenarioStep map[string]interface{}

// ScrapeConfig holds all the parameters for a scrape request.
type ScrapeConfig struct {
	URL                         string
	Method                      string // GET, POST, PUT, PATCH
	Body                        string
	Data                        map[string]interface{}
	Headers                     map[string]string
	Cookies                     map[string]string
	Country                     string
	ProxyPool                   ProxyPool
	RenderJS                    bool
	ASP                         bool
	Cache                       bool
	CacheTTL                    int
	CacheClear                  bool
	Timeout                     int
	Retry                       bool
	Session                     string
	SessionStickyProxy          bool
	Tags                        []string
	Webhook                     string
	Debug                       bool
	SSL                         bool
	DNS                         bool
	CorrelationID               string
	Format                      Format
	FormatOptions               []FormatOption
	ExtractionTemplate          string
	ExtractionEphemeralTemplate map[string]interface{}
	ExtractionPrompt            string
	ExtractionModel             string
	WaitForSelector             string
	RenderingWait               int
	AutoScroll                  bool
	Screenshots                 map[string]string
	ScreenshotFlags             []ScreenshotFlag
	JS                          string
	JSScenario                  []JSScenarioStep
	OS                          string
	Lang                        []string
}

// toAPIParams converts the ScrapeConfig into URL parameters for the Scrapfly API.
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
				params.Set(fmt.Sprintf("screenshots[%s]", name), value)
			}
		}
		if len(c.ScreenshotFlags) > 0 {
			var flags []string
			for _, flag := range c.ScreenshotFlags {
				flags = append(flags, string(flag))
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
		formatVal := string(c.Format)
		if len(c.FormatOptions) > 0 {
			var opts []string
			for _, opt := range c.FormatOptions {
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
		params.Set(fmt.Sprintf("headers[%s]", strings.ToLower(key)), value)
	}
	if len(c.Cookies) > 0 {
		var cookieParts []string
		for name, value := range c.Cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", name, value))
		}
		cookieHeader := strings.Join(cookieParts, "; ")

		if existingCookie, ok := c.Headers["cookie"]; ok {
			params.Set("headers[cookie]", existingCookie+"; "+cookieHeader)
		} else {
			params.Set("headers[cookie]", cookieHeader)
		}
	}

	return params, nil
}

// processBody handles the Data and Body fields for POST/PUT/PATCH requests.
func (c *ScrapeConfig) processBody() error {
	method := strings.ToUpper(c.Method)
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
