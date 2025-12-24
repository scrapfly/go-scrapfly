package scrapfly

import (
	"fmt"
	"net/url"
	"strings"
)

// ScreenshotFormat defines the image format for screenshots.
type ScreenshotFormat string

// Available image formats for screenshot capture.
const (
	// FormatJPG captures screenshots in JPEG format (smaller file size, lossy compression).
	FormatJPG ScreenshotFormat = "jpg"
	// FormatPNG captures screenshots in PNG format (larger file size, lossless compression).
	FormatPNG ScreenshotFormat = "png"
	// FormatWEBP captures screenshots in WebP format (modern format with good compression).
	FormatWEBP ScreenshotFormat = "webp"
	// FormatGIF captures screenshots in GIF format (animated screenshots support).
	FormatGIF ScreenshotFormat = "gif"
)

// ScreenshotOption defines options to customize screenshot capture behavior.
type ScreenshotOption string

// Available options for customizing screenshot capture.
const (
	// OptionLoadImages enables image loading (disabled by default for performance).
	OptionLoadImages ScreenshotOption = "load_images"
	// OptionDarkMode enables dark mode rendering.
	OptionDarkMode ScreenshotOption = "dark_mode"
	// OptionBlockBanners blocks cookie banners and similar overlays.
	OptionBlockBanners ScreenshotOption = "block_banners"
	// OptionPrintMediaFormat uses print media CSS for rendering.
	OptionPrintMediaFormat ScreenshotOption = "print_media_format"
)

// ScreenshotConfig configures a screenshot capture request to the Scrapfly API.
//
// This struct contains all available options for customizing screenshot behavior,
// including format, resolution, capture area, and rendering options.
//
// Example:
//
//	config := &scrapfly.ScreenshotConfig{
//	    URL:        "https://example.com",
//	    Format:     scrapfly.FormatPNG,
//	    Capture:    "fullpage",
//	    Resolution: "1920x1080",
//	    Options:    []scrapfly.ScreenshotOption{scrapfly.OptionBlockBanners},
//	}
type ScreenshotConfig struct {
	// URL is the target URL to capture (required).
	URL string
	// Format specifies the image format (jpg, png, webp, gif).
	Format ScreenshotFormat
	// Capture defines what to capture: "fullpage" for entire page, or a CSS selector for specific element.
	Capture string
	// Resolution sets the viewport size (e.g., "1920x1080").
	Resolution string
	// Country specifies the proxy country code (e.g., "us", "uk", "de").
	Country string
	// Timeout sets the maximum time in milliseconds to wait for the request.
	Timeout int
	// RenderingWait is additional wait time in milliseconds after page load.
	RenderingWait int
	// WaitForSelector waits for a CSS selector to appear before capturing.
	WaitForSelector string
	// Options are additional screenshot options (dark mode, block banners, etc.).
	Options []ScreenshotOption
	// AutoScroll automatically scrolls the page to load lazy content.
	AutoScroll bool
	// JS is custom JavaScript code to execute before capturing.
	JS string
	// Cache enables response caching.
	Cache bool
	// CacheTTL sets the cache time-to-live in seconds.
	CacheTTL int
	// CacheClear forces cache refresh for this request.
	CacheClear bool
	// Webhook is the name of a webhook to call after the request completes.
	Webhook string
	// VisionDeficiencyType specifies the type of vision deficiency to simulate.
	// see https://scrapfly.io/docs/screenshot-api/accessibility#vision_deficiency
	VisionDeficiencyType VisionDeficiencyType
}

// toAPIParams converts the ScreenshotConfig into URL parameters for the Scrapfly API.
// This is an internal method used by the Client to prepare API requests.
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

	if c.VisionDeficiencyType != "" {
		params.Set("vision_deficiency", string(c.VisionDeficiencyType))
	}

	return params, nil
}
