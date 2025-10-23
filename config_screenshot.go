package scrapfly

import (
	"fmt"
	"net/url"
	"strings"
)

// ScreenshotFormat defines the format of the screenshot image.
type ScreenshotFormat string

const (
	FormatJPG  ScreenshotFormat = "jpg"
	FormatPNG  ScreenshotFormat = "png"
	FormatWEBP ScreenshotFormat = "webp"
	FormatGIF  ScreenshotFormat = "gif"
)

// ScreenshotOption defines options to customize screenshot behavior.
type ScreenshotOption string

const (
	OptionLoadImages       ScreenshotOption = "load_images"
	OptionDarkMode         ScreenshotOption = "dark_mode"
	OptionBlockBanners     ScreenshotOption = "block_banners"
	OptionPrintMediaFormat ScreenshotOption = "print_media_format"
)

// ScreenshotConfig holds parameters for a screenshot request.
type ScreenshotConfig struct {
	URL             string
	Format          ScreenshotFormat
	Capture         string // e.g., "fullpage" or a CSS selector
	Resolution      string // e.g., "1920x1080"
	Country         string
	Timeout         int
	RenderingWait   int
	WaitForSelector string
	Options         []ScreenshotOption
	AutoScroll      bool
	JS              string
	Cache           bool
	CacheTTL        int
	CacheClear      bool
	Webhook         string
}

// toAPIParams converts the ScreenshotConfig into URL parameters.
func (c *ScreenshotConfig) toAPIParams() (url.Values, error) {
	params := url.Values{}

	if c.URL == "" {
		return nil, fmt.Errorf("%w: URL is required", ErrScreenshotConfig)
	}
	params.Set("url", c.URL)

	if c.Format != "" {
		params.Set("format", string(c.Format))
	}
	if c.Capture != "" {
		params.Set("capture", c.Capture)
	}
	if c.Resolution != "" {
		params.Set("resolution", c.Resolution)
	}
	if c.Country != "" {
		params.Set("country", c.Country)
	}
	if c.Timeout > 0 {
		params.Set("timeout", fmt.Sprint(c.Timeout))
	}
	if c.RenderingWait > 0 {
		params.Set("rendering_wait", fmt.Sprint(c.RenderingWait))
	}
	if c.WaitForSelector != "" {
		params.Set("wait_for_selector", c.WaitForSelector)
	}
	if c.AutoScroll {
		params.Set("auto_scroll", "true")
	}
	if c.JS != "" {
		params.Set("js", urlSafeB64Encode(c.JS))
	}

	if len(c.Options) > 0 {
		var opts []string
		for _, opt := range c.Options {
			opts = append(opts, string(opt))
		}
		params.Set("options", strings.Join(opts, ","))
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

	if c.Webhook != "" {
		params.Set("webhook_name", c.Webhook)
	}

	return params, nil
}
