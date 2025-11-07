package scrapfly

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// ExtractionModel defines the type of extraction model to use for extraction.
// see https://scrapfly.io/docs/extraction-api/automatic-ai#models
type ExtractionModel string

const (
	ExtractionModelArticle                   ExtractionModel = "article"
	ExtractionModelEvent                     ExtractionModel = "event"
	ExtractionModelFoodRecipe                ExtractionModel = "food_recipe"
	ExtractionModelHotel                     ExtractionModel = "hotel"
	ExtractionModelHotelListing              ExtractionModel = "hotel_listing"
	ExtractionModelJobListing                ExtractionModel = "job_listing"
	ExtractionModelJobPosting                ExtractionModel = "job_posting"
	ExtractionModelOrganization              ExtractionModel = "organization"
	ExtractionModelProduct                   ExtractionModel = "product"
	ExtractionModelProductListing            ExtractionModel = "product_listing"
	ExtractionModelRealEstateProperty        ExtractionModel = "real_estate_property"
	ExtractionModelRealEstatePropertyListing ExtractionModel = "real_estate_property_listing"
	ExtractionModelReviewList                ExtractionModel = "review_list"
	ExtractionModelSearchEngineResults       ExtractionModel = "search_engine_results"
	ExtractionModelSocialMediaPost           ExtractionModel = "social_media_post"
	ExtractionModelSoftware                  ExtractionModel = "software"
	ExtractionModelStock                     ExtractionModel = "stock"
	ExtractionModelVehicleAd                 ExtractionModel = "vehicle_ad"
	ExtractionModelVehicleAdListing          ExtractionModel = "vehicle_ad_listing"
)

func (f ExtractionModel) Enum() []ExtractionModel {
	return []ExtractionModel{ExtractionModelArticle, ExtractionModelEvent, ExtractionModelFoodRecipe, ExtractionModelHotel, ExtractionModelHotelListing, ExtractionModelJobListing, ExtractionModelJobPosting, ExtractionModelOrganization, ExtractionModelProduct, ExtractionModelProductListing, ExtractionModelRealEstateProperty, ExtractionModelRealEstatePropertyListing, ExtractionModelReviewList, ExtractionModelSearchEngineResults, ExtractionModelSocialMediaPost, ExtractionModelSoftware, ExtractionModelStock, ExtractionModelVehicleAd, ExtractionModelVehicleAdListing}
}

func (f ExtractionModel) AnyEnum() []any {
	return []any{ExtractionModelArticle, ExtractionModelEvent, ExtractionModelFoodRecipe, ExtractionModelHotel, ExtractionModelHotelListing, ExtractionModelJobListing, ExtractionModelJobPosting, ExtractionModelOrganization, ExtractionModelProduct, ExtractionModelProductListing, ExtractionModelRealEstateProperty, ExtractionModelRealEstatePropertyListing, ExtractionModelReviewList, ExtractionModelSearchEngineResults, ExtractionModelSocialMediaPost, ExtractionModelSoftware, ExtractionModelStock, ExtractionModelVehicleAd, ExtractionModelVehicleAdListing}
}
func (f ExtractionModel) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_extraction_model"
}

func (f ExtractionModel) IsValid() bool {
	return IsValidEnumType(f)
}

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

func (f ProxyPool) AnyEnum() []any {
	return []any{PublicDataCenterPool, PublicResidentialPool}
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
func (f ScreenshotFlag) AnyEnum() []any {
	return []any{LoadImages, DarkMode, BlockBanners, PrintMediaFormat, HighQuality}
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

func (f Format) AnyEnum() []any {
	return []any{FormatJSON, FormatText, FormatMarkdown, FormatCleanHTML, FormatRaw}
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
func (f FormatOption) AnyEnum() []any {
	return []any{NoLinks, NoImages, OnlyContent}
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

func (f HttpMethod) AnyEnum() []any {
	return []any{HttpMethodGet, HttpMethodPost, HttpMethodPut, HttpMethodPatch, HttpMethodOptions}
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

type Enumerable[T fmt.Stringer] interface {
	Enum() []T
	AnyEnum() []any
}

func IsValidEnumType[T fmt.Stringer](f T) bool {
	return !strings.HasPrefix(f.String(), "invalid")
}

func GetEnumFor[V Enumerable[T], T fmt.Stringer]() []T {
	var v V
	return v.Enum()
}

func GetAnyEnumFor[V Enumerable[T], T fmt.Stringer]() []any {
	var v V
	return v.AnyEnum()
}
