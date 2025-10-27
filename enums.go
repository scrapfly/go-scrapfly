package scrapfly

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// ProxyPool represents the type of proxy pool to use for scraping.
type ProxyPool string

// Available proxy pool types for Scrapfly API requests.
const (
	// PublicDataCenterPool uses datacenter proxies. Fast and reliable but easier to detect.
	PublicDataCenterPool ProxyPool = "public_datacenter_pool"
	// PublicResidentialPool uses residential proxies. More expensive but harder to detect.
	PublicResidentialPool ProxyPool = "public_residential_pool"
)

func (f ProxyPool) Enum() []ProxyPool {
	return []ProxyPool{PublicDataCenterPool, PublicResidentialPool}
}

func (f ProxyPool) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_proxy_pool"
}

func (f *ProxyPool) IsValid() bool {
	return IsValidEnumType(f)
}

// ScreenshotFlag defines options for screenshot behavior when using Screenshots parameter.
type ScreenshotFlag string

// Available screenshot flags for customizing screenshot capture.
const (
	// LoadImages enables image loading for screenshots (disabled by default for performance).
	LoadImages ScreenshotFlag = "load_images"
	// DarkMode enables dark mode rendering.
	DarkMode ScreenshotFlag = "dark_mode"
	// BlockBanners blocks cookie banners and similar overlays.
	BlockBanners ScreenshotFlag = "block_banners"
	// PrintMediaFormat uses print media CSS for rendering.
	PrintMediaFormat ScreenshotFlag = "print_media_format"
	// HighQuality captures screenshots at higher quality settings.
	HighQuality ScreenshotFlag = "high_quality"
)

func (f ScreenshotFlag) Enum() []ScreenshotFlag {
	return []ScreenshotFlag{LoadImages, DarkMode, BlockBanners, PrintMediaFormat, HighQuality}
}
func (f ScreenshotFlag) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_screenshot_flag"
}

func (f ScreenshotFlag) IsValid() bool {
	return IsValidEnumType(f)
}

// Format defines the format for the scraped content response.
type Format string

// Available content formats for scrape responses.
const (
	// FormatJSON returns content structured as JSON.
	FormatJSON Format = "json"
	// FormatText returns plain text content with HTML tags stripped.
	FormatText Format = "text"
	// FormatMarkdown converts HTML to Markdown format.
	FormatMarkdown Format = "markdown"
	// FormatCleanHTML returns cleaned and normalized HTML.
	FormatCleanHTML Format = "clean_html"
	// FormatRaw returns the raw HTML content without any processing.
	FormatRaw Format = "raw"
)

func (f Format) Enum() []Format {
	return []Format{FormatJSON, FormatText, FormatMarkdown, FormatCleanHTML, FormatRaw}
}

func (f Format) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_format"
}

func (f Format) IsValid() bool {
	return IsValidEnumType(f)
}

// FormatOption defines additional options for content formatting.
type FormatOption string

// Available format options that can be combined with Format settings.
const (
	// NoLinks removes all links from the formatted content.
	NoLinks FormatOption = "no_links"
	// NoImages removes all images from the formatted content.
	NoImages FormatOption = "no_images"
	// OnlyContent extracts only the main content, removing headers, footers, and navigation.
	OnlyContent FormatOption = "only_content"
)

func (f FormatOption) Enum() []FormatOption {
	return []FormatOption{NoLinks, NoImages, OnlyContent}
}
func (f FormatOption) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_format_option"
}

func (f FormatOption) IsValid() bool {
	return IsValidEnumType(f)
}

type HttpMethod string

const (
	HttpMethodGet     HttpMethod = http.MethodGet
	HttpMethodPost    HttpMethod = http.MethodPost
	HttpMethodPut     HttpMethod = http.MethodPut
	HttpMethodPatch   HttpMethod = http.MethodPatch
	HttpMethodOptions HttpMethod = http.MethodOptions
	//HttpMethodConnect HttpMethod = http.MethodConnect 404 on scrape endpoint
	//HttpMethodTrace HttpMethod = http.MethodTrace 404 on scrape endpoint
	//HttpMethodDelete HttpMethod = http.MethodDelete 404 on scrape endpoint
	//HttpMethodHead HttpMethod = http.MethodHead will actually HEAD on scrape endpoint instead of target, by design, so will never work
)

func (f HttpMethod) Enum() []HttpMethod {
	return []HttpMethod{HttpMethodGet, HttpMethodPost, HttpMethodPut, HttpMethodPatch, HttpMethodOptions}
}

func (f HttpMethod) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_http_method"
}

func (f HttpMethod) IsValid() bool {
	return IsValidEnumType(f)
}

type Enumerable interface {
	Enum() []Enumerable
}

func IsValidEnumType[T fmt.Stringer](f T) bool {
	return !strings.HasPrefix(f.String(), "invalid")
}

func GetEnumFor[T Enumerable]() []Enumerable {
	var v T
	return v.Enum()
}
