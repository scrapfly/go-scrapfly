package scrapfly

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// VerifyAPIKeyResult represents the result of an API key verification.
type VerifyAPIKeyResult struct {
	// Valid indicates whether the API key is valid.
	Valid bool `json:"valid"`
}

// ScrapeResult represents the complete response from a scrape request.
//
// It contains the scraped content, metadata, configuration, and context
// information about the request. The result includes details about the
// upstream response, proxy used, cost, and more.
type ScrapeResult struct {
	// Config contains the configuration used for this scrape request.
	Config ConfigData `json:"config"`
	// Context contains metadata about the request execution.
	Context ContextData `json:"context"`
	// Result contains the scraped content and response data.
	Result ResultData `json:"result"`
	// UUID is the unique identifier for this scrape request.
	UUID string `json:"uuid"`

	selector *goquery.Document // For lazy loading
}

// Selector provides a goquery document for parsing HTML content.
//
// The selector is lazy-loaded and cached, making it efficient to call
// multiple times. It can only be used with HTML content.
//
// Example:
//
//	result, err := client.Scrape(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	doc, err := result.Selector()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	title := doc.Find("title").First().Text()
//	fmt.Println(title)
func (r *ScrapeResult) Selector() (*goquery.Document, error) {
	if r.selector != nil {
		return r.selector, nil
	}

	if !strings.Contains(r.Result.ContentType, "text/html") {
		return nil, fmt.Errorf("%w: cannot use selector on non-html content-type, got %s", ErrContentType, r.Result.ContentType)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(r.Result.Content))
	if err != nil {
		return nil, err
	}

	r.selector = doc
	return r.selector, nil
}

// ExtractionResult represents the result of a data extraction request.
type ExtractionResult struct {
	// Data contains the extracted structured data.
	Data interface{} `json:"data"`
	// ContentType is the content type of the extracted data.
	ContentType string `json:"content_type"`
	// DataQuality indicates the quality/confidence of the extraction (if available).
	DataQuality string `json:"data_quality,omitempty"`
}

// errorResponse is used to unmarshal generic API errors.
type errorResponse struct {
	Message  string `json:"message"`
	ErrorID  string `json:"error_id"`
	HTTPCode int    `json:"http_code"`
	Code     string `json:"code"`
}

// --- Detailed Data Structures ---

// ConfigData contains the configuration that was used for a scrape request.
// This mirrors the 'config' object in the API response.
type ConfigData struct {
	URL                string            `json:"url"`
	Method             string            `json:"method"`
	Country            *string           `json:"country"`
	RenderJS           bool              `json:"render_js"`
	Cache              bool              `json:"cache"`
	CacheClear         bool              `json:"cache_clear"`
	CacheTTL           int               `json:"cache_ttl"`
	SSL                bool              `json:"ssl"`
	DNS                bool              `json:"dns"`
	ASP                bool              `json:"asp"`
	Debug              bool              `json:"debug"`
	ProxyPool          string            `json:"proxy_pool"`
	Session            *string           `json:"session"`
	SessionStickyProxy bool              `json:"session_sticky_proxy"`
	Tags               []string          `json:"tags"`
	CorrelationID      *string           `json:"correlation_id"`
	Body               *string           `json:"body"`
	Headers            map[string]string `json:"headers"`
	JS                 *string           `json:"js"`
	RenderingWait      int               `json:"rendering_wait"`
	WaitForSelector    *string           `json:"wait_for_selector"`
	Screenshots        map[string]string `json:"screenshots"`
	WebhookName        *string           `json:"webhook_name"`
	Timeout            int               `json:"timeout"`
	JSScenario         interface{}       `json:"js_scenario"`
	Extract            interface{}       `json:"extract"`
	Lang               []string          `json:"lang"`
	OS                 *string           `json:"os"`
	AutoScroll         bool              `json:"auto_scroll"`
	Env                string            `json:"env"`
	Origin             string            `json:"origin"`
	Project            string            `json:"project"`
	UserUUID           string            `json:"user_uuid"`
	UUID               string            `json:"uuid"`
}

// ContextData contains metadata about the scrape request execution.
// This includes proxy information, costs, cache status, and more.
type ContextData struct {
	ASP               interface{}  `json:"asp"`
	BandwidthConsumed int          `json:"bandwidth_consumed"`
	Cache             CacheContext `json:"cache"`
	Cookies           []Cookie     `json:"cookies"`
	Cost              CostContext  `json:"cost"`
	CreatedAt         string       `json:"created_at"`
	Debug             DebugContext `json:"debug"`
	Env               string       `json:"env"`
	//Fingerprint       string            `json:"fingerprint"`
	Fingerprint      interface{}       `json:"fingerprint"`
	Headers          map[string]string `json:"headers"`
	IsXMLHTTPRequest bool              `json:"is_xml_http_request"`
	Job              interface{}       `json:"job"`
	Lang             interface{}       `json:"lang"` // []string or string
	OS               OSContext         `json:"os"`
	Project          string            `json:"project"`
	Proxy            ProxyContext      `json:"proxy"`
	Redirects        interface{}       `json:"redirects"` // []string or string
	Retry            int               `json:"retry"`
	Schedule         interface{}       `json:"schedule"`
	Session          interface{}       `json:"session"`
	Spider           interface{}       `json:"spider"`
	Throttler        interface{}       `json:"throttler"`
	URI              URIContext        `json:"uri"`
	URL              string            `json:"url"`
	Webhook          interface{}       `json:"webhook"`
}

// ResultData contains the scraped content and response information.
// This is the main data from the scrape request including HTML content,
// status codes, headers, cookies, and more.
type ResultData struct {
	BrowserData     BrowserData            `json:"browser_data"`
	Content         string                 `json:"content"`
	ContentEncoding string                 `json:"content_encoding"`
	ContentType     string                 `json:"content_type"`
	Cookies         []Cookie               `json:"cookies"`
	Data            interface{}            `json:"data"`
	DNS             interface{}            `json:"dns"`
	Duration        float64                `json:"duration"`
	Error           *APIErrorDetails       `json:"error"`
	Format          string                 `json:"format"`
	IFrames         []IFrame               `json:"iframes"`
	LogURL          string                 `json:"log_url"`
	Reason          string                 `json:"reason"`
	RequestHeaders  map[string]string      `json:"request_headers"`
	ResponseHeaders map[string]interface{} `json:"response_headers"` // Can be string or []string
	Screenshots     map[string]Screenshot  `json:"screenshots"`
	Size            int                    `json:"size"`
	SSL             interface{}            `json:"ssl"`
	Status          string                 `json:"status"`
	StatusCode      int                    `json:"status_code"`
	Success         bool                   `json:"success"`
	URL             string                 `json:"url"`
	ExtractedData   *ExtractionResult      `json:"extracted_data"`
}

// --- Nested Structures for Context and Result ---

// CacheContext contains information about cache usage for the request.
type CacheContext struct {
	State string      `json:"state"`
	Entry interface{} `json:"entry"`
}

// CostDetail represents a single cost item for a scrape request.
type CostDetail struct {
	Amount      int    `json:"amount"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

// CostContext contains the cost breakdown for a scrape request.
type CostContext struct {
	Details []CostDetail `json:"details"`
	Total   int          `json:"total"`
}

// DebugContext contains URLs for debugging the request.
type DebugContext struct {
	ResponseURL   string      `json:"response_url"`
	ScreenshotURL interface{} `json:"screenshot_url"`
}

// OSContext contains information about the operating system used for the request.
type OSContext struct {
	Distribution string `json:"distribution"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Version      string `json:"version"`
}

// ProxyContext contains information about the proxy used for the request.
type ProxyContext struct {
	Country  string `json:"country"`
	Identity string `json:"identity"`
	Network  string `json:"network"`
	Pool     string `json:"pool"`
}

// URIContext contains parsed URI information about the requested URL.
type URIContext struct {
	BaseURL    string      `json:"base_url"`
	Fragment   interface{} `json:"fragment"`
	Host       string      `json:"host"`
	Params     interface{} `json:"params"`
	Port       int         `json:"port"`
	Query      interface{} `json:"query"`
	RootDomain string      `json:"root_domain"`
	Scheme     string      `json:"scheme"`
}

// BrowserData contains data collected from the browser during JavaScript rendering.
type BrowserData struct {
	JSEvaluationResult *string                `json:"javascript_evaluation_result"`
	JSScenario         interface{}            `json:"js_scenario"`
	LocalStorageData   map[string]interface{} `json:"local_storage_data"`
	SessionStorageData map[string]interface{} `json:"session_storage_data"`
	Websockets         []interface{}          `json:"websockets"`
	XHRCall            []interface{}          `json:"xhr_call"`
	Attachments        []Attachment           `json:"attachments"`
}

type Attachment struct {
	Content           string `json:"content"`
	ContentType       string `json:"content_type"`
	Filename          string `json:"filename"`
	ID                string `json:"id"`
	Size              int    `json:"size"`
	State             string `json:"state"`
	SuggestedFilename string `json:"suggested_filename"`
	URL               string `json:"url"`

	data []byte
}

// Cookie represents an HTTP cookie.
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Expires  string `json:"expires"`
	Path     string `json:"path"`
	Comment  string `json:"comment"`
	Domain   string `json:"domain"`
	MaxAge   int    `json:"max_age"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"http_only"`
	Version  string `json:"version"`
	Size     int    `json:"size"`
}

// APIErrorDetails contains detailed error information from the API.
type APIErrorDetails struct {
	Code      string            `json:"code"`
	HTTPCode  int               `json:"http_code"`
	Links     map[string]string `json:"links"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	DocURL    string            `json:"doc_url"`
}

// IFrame represents an iframe found in the page.
type IFrame struct {
	URL     string     `json:"url"`
	URI     URIContext `json:"uri"`
	Content string     `json:"content"`
}

// Screenshot represents a screenshot captured during rendering.
type Screenshot struct {
	// CSSSelector is the CSS selector of the element to capture. If Format == fullpage, this will be nil
	CSSSelector *string `json:"css_selector"`
	// Extension is the file extension (jpg, png, webp, gif)
	Extension string `json:"extension"` // Always jpg when request from scraping api.
	// Format is the format of the screenshot (fullpage, element)
	Format string `json:"format"`
	// Size is the size of the screenshot in bytes
	Size int `json:"size"`
	// URL is the URL to retrieve the screenshot from
	URL string `json:"url"`

	// Name is the name of the screenshot retrieved from the API response
	Name string `json:"-"`

	image []byte
}

// Image returns the screenshot data as a byte slice.
func (s *Screenshot) Image() ([]byte, error) {
	if s.image != nil {
		return s.image, nil
	}
	resp, err := http.Get(s.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	s.image, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return s.image, nil
}

// Data returns the attachment data as a byte slice.
func (a *Attachment) Data() ([]byte, error) {
	if a.data != nil {
		return a.data, nil
	}
	resp, err := http.Get(a.Content)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	a.data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return a.data, nil
}

// Save saves a scraped attachment result to disk.
//
// Parameters:
//   - savePath: Optional directory path where to save the file (defaults to current directory)
//     (if savePath does not exists, it will be created in a best effort basis)
//
// it is named as the filename of the attachment
// Returns the full path to the saved file.
//
// Example:
//
//	filePath, err := a.Save("./attachments")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Attachment %s saved to: %s\n", a.Filename, filePath)
func (a *Attachment) Save(savePath ...string) (string, error) {
	if a.data == nil {
		_, err := a.Data()
		if err != nil {
			return "", err
		}
	}
	dir := "."
	if len(savePath) > 0 {
		dir = savePath[0]
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filePath := filepath.Join(dir, fmt.Sprintf("%s", a.Filename))
	err := os.WriteFile(filePath, a.data, 0644)
	return filePath, err
}

// Save saves a scraped screenshot result to disk.
//
// Parameters:
//   - savePath: Optional directory path where to save the file (defaults to current directory)
//     (if savePath does not exists, it will be created in a best effort basis)
//
// it is named as the name of the screenshot as set in config and retrieved from the API response
// Returns the full path to the saved file.
//
// Example:
//
//	filePath, err := s.Save("./screenshots")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Screenshot saved to: %s\n", filePath)
func (s *Screenshot) Save(savePath ...string) (string, error) {
	if s.image == nil {
		_, err := s.Image()
		if err != nil {
			return "", err
		}
	}
	dir := "."
	if len(savePath) > 0 {
		dir = savePath[0]
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filePath := filepath.Join(dir, fmt.Sprintf("%s.%s", s.Name, s.Extension))
	err := os.WriteFile(filePath, s.image, 0644)
	return filePath, err
}

// SaveScreenshots is a shortcut to save all screenshots to disk
//
// Parameters:
//   - savePath: Optional directory path where to save the files (defaults to current directory)
//     (if savePath does not exists, it will be created in a best effort basis)
//
// Returns the full paths to the saved files.
//
// Example:
//
//	paths, err := r.SaveScreenshots("./screenshots")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, path := range paths {
//		fmt.Printf("Screenshot saved to: %s\n", path)
//	}
func (r *ScrapeResult) SaveScreenshots(savePath ...string) ([]string, error) {
	paths := []string{}
	for _, screenshot := range r.Result.Screenshots {
		filePath, err := screenshot.Save(savePath...)
		if err != nil {
			return nil, err
		}
		paths = append(paths, filePath)
	}
	return paths, nil
}

// SaveAttachments is a shortcut to save all attachments to disk
//
// Parameters:
//   - savePath: Optional directory path where to save the files (defaults to current directory)
//     (if savePath does not exists, it will be created in a best effort basis)
//
// Returns the full paths to the saved files.
//
// Example:
//
//	paths, err := r.SaveAttachments("./attachments")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, path := range paths {
//		fmt.Printf("Attachment saved to: %s\n", path)
//	}
func (r *ScrapeResult) SaveAttachments(savePath ...string) ([]string, error) {
	paths := []string{}
	for _, attachment := range r.Result.BrowserData.Attachments {
		filePath, err := attachment.Save(savePath...)
		if err != nil {
			return nil, err
		}
		paths = append(paths, filePath)
	}
	return paths, nil
}
