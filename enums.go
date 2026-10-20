package scrapfly

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

type VisionDeficiencyType string

const (
	VisionDeficiencyTypeNone            VisionDeficiencyType = "none"
	VisionDeficiencyTypeDeuteranopia    VisionDeficiencyType = "deuteranopia"
	VisionDeficiencyTypeProtanopia      VisionDeficiencyType = "protanopia"
	VisionDeficiencyTypeTritanopia      VisionDeficiencyType = "tritanopia"
	VisionDeficiencyTypeAchromatopsia   VisionDeficiencyType = "achromatopsia"
	VisionDeficiencyTypeBlurredVision   VisionDeficiencyType = "blurredVision"
	VisionDeficiencyTypeReducedContrast VisionDeficiencyType = "reducedContrast"
)

func (f VisionDeficiencyType) Enum() []VisionDeficiencyType {
	return []VisionDeficiencyType{VisionDeficiencyTypeNone, VisionDeficiencyTypeDeuteranopia, VisionDeficiencyTypeProtanopia, VisionDeficiencyTypeTritanopia, VisionDeficiencyTypeAchromatopsia, VisionDeficiencyTypeBlurredVision}
}

func (f VisionDeficiencyType) AnyEnum() []any {
	return []any{VisionDeficiencyTypeNone, VisionDeficiencyTypeDeuteranopia, VisionDeficiencyTypeProtanopia, VisionDeficiencyTypeTritanopia, VisionDeficiencyTypeAchromatopsia, VisionDeficiencyTypeBlurredVision}
}
func (f VisionDeficiencyType) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_vision_deficiency_type"
}

func (f VisionDeficiencyType) IsValid() bool {
	return IsValidEnumType(f)
}

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
	ExtractionModelNone                      ExtractionModel = ""
)

func (f ExtractionModel) Enum() []ExtractionModel {
	return []ExtractionModel{ExtractionModelArticle, ExtractionModelEvent, ExtractionModelFoodRecipe, ExtractionModelHotel, ExtractionModelHotelListing, ExtractionModelJobListing, ExtractionModelJobPosting, ExtractionModelOrganization, ExtractionModelProduct, ExtractionModelProductListing, ExtractionModelRealEstateProperty, ExtractionModelRealEstatePropertyListing, ExtractionModelReviewList, ExtractionModelSearchEngineResults, ExtractionModelSocialMediaPost, ExtractionModelSoftware, ExtractionModelStock, ExtractionModelVehicleAd, ExtractionModelVehicleAdListing, ExtractionModelNone}
}

func (f ExtractionModel) AnyEnum() []any {
	return []any{ExtractionModelArticle, ExtractionModelEvent, ExtractionModelFoodRecipe, ExtractionModelHotel, ExtractionModelHotelListing, ExtractionModelJobListing, ExtractionModelJobPosting, ExtractionModelOrganization, ExtractionModelProduct, ExtractionModelProductListing, ExtractionModelRealEstateProperty, ExtractionModelRealEstatePropertyListing, ExtractionModelReviewList, ExtractionModelSearchEngineResults, ExtractionModelSocialMediaPost, ExtractionModelSoftware, ExtractionModelStock, ExtractionModelVehicleAd, ExtractionModelVehicleAdListing, ExtractionModelNone}
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

// ProxyPool is the proxy_pool parameter of the Web Scraping API, the Crawler
// and the Screenshot API: ProxyPoolBasic, ProxyPoolUnblocker,
// ProxyPoolUnblockerUltra or ProxyPoolAuto, each of which sets both the
// proxies and the Unblocker, or PublicTorPool, the Tor network. The Screenshot
// API takes the first four and ignores any other value.
//
// The pool names written before the proxy pools (PublicDataCenterPool,
// PublicResidentialPool) are still accepted. The migration guide covers them:
// https://scrapfly.io/new-pricing#legacy-spellings
type ProxyPool string

// Proxy pools.
const (
	// ProxyPoolBasic runs on datacenter proxies with the Unblocker off.
	ProxyPoolBasic ProxyPool = "basic"
	// ProxyPoolUnblocker runs on residential proxies with the Unblocker
	// (anti-bot bypass) on.
	ProxyPoolUnblocker ProxyPool = "unblocker"
	// ProxyPoolUnblockerUltra runs on residential proxies with the Unblocker
	// on. It is the only pool accepted by the targets and anti-bot
	// protections that have a price of their own: those refuse basic and
	// unblocker with ERR::PROXY::UNBLOCKER_ULTRA_REQUIRED (HTTP 422, not
	// billed).
	ProxyPoolUnblockerUltra ProxyPool = "unblocker_ultra"
	// ProxyPoolAuto lets the API pick the pool before the request, from the
	// target's profile: the Unblocker on datacenter proxies, or the pool the
	// target needs. Accepted on every target, the ones that require
	// ProxyPoolUnblockerUltra included, where their price is added; priced as
	// the pool it runs on.
	ProxyPoolAuto ProxyPool = "auto"
	// PublicTorPool routes egress through the Tor network, the only pool that
	// reaches .onion hidden services; also used for OSINT.
	PublicTorPool ProxyPool = "public_tor_pool"
)

// Pool names written before the proxy pools, still accepted. The migration
// guide covers them: https://scrapfly.io/new-pricing#legacy-spellings
const (
	// PublicDataCenterPool is the former name of the datacenter pool.
	PublicDataCenterPool ProxyPool = "public_datacenter_pool"
	// PublicResidentialPool is the former name of the residential pool.
	PublicResidentialPool ProxyPool = "public_residential_pool"
)

func (f ProxyPool) Enum() []ProxyPool {
	return []ProxyPool{ProxyPoolBasic, ProxyPoolUnblocker, ProxyPoolUnblockerUltra, ProxyPoolAuto, PublicTorPool, PublicDataCenterPool, PublicResidentialPool}
}

func (f ProxyPool) String() string {
	if slices.Contains(f.Enum(), f) {
		return string(f)
	}
	return "invalid_proxy_pool"
}

func (f ProxyPool) AnyEnum() []any {
	return []any{ProxyPoolBasic, ProxyPoolUnblocker, ProxyPoolUnblockerUltra, ProxyPoolAuto, PublicTorPool, PublicDataCenterPool, PublicResidentialPool}
}
func (f ProxyPool) IsValid() bool {
	return IsValidEnumType(f)
}

// IsLegacy reports whether f is one of the pool names written before the
// proxy pools (PublicDataCenterPool, PublicResidentialPool). PublicTorPool is
// not one: the Tor pool keeps its name.
func (f ProxyPool) IsLegacy() bool {
	switch f {
	case PublicDataCenterPool, PublicResidentialPool:
		return true
	}
	return false
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
	HttpMethodHead    HttpMethod = http.MethodHead
	//HttpMethodConnect HttpMethod = http.MethodConnect 404 on scrape endpoint
	//HttpMethodTrace HttpMethod = http.MethodTrace 404 on scrape endpoint
	//HttpMethodDelete HttpMethod = http.MethodDelete rejected by the scrape endpoint; it allows GET, POST, PUT, PATCH, OPTIONS, HEAD only
)

func (f HttpMethod) Enum() []HttpMethod {
	return []HttpMethod{HttpMethodGet, HttpMethodPost, HttpMethodPut, HttpMethodPatch, HttpMethodOptions, HttpMethodHead}
}

func (f HttpMethod) AnyEnum() []any {
	return []any{HttpMethodGet, HttpMethodPost, HttpMethodPut, HttpMethodPatch, HttpMethodOptions, HttpMethodHead}
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
