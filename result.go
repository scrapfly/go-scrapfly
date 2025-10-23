package scrapfly

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type VerifyAPIKeyResult struct {
	Valid bool `json:"valid"`
}

// ScrapeResult represents the result of a scrape call.
type ScrapeResult struct {
	Config  ConfigData  `json:"config"`
	Context ContextData `json:"context"`
	Result  ResultData  `json:"result"`
	UUID    string      `json:"uuid"`

	selector *goquery.Document // For lazy loading
}

// Selector provides a go-query document for parsing the HTML content.
// It is lazy-loaded and cached.
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

// ScreenshotResult represents a screenshot taken by the API.
type ScreenshotResult struct {
	Image    []byte
	Metadata ScreenshotMetadata
}

// ScreenshotMetadata contains metadata about the screenshot.
type ScreenshotMetadata struct {
	ExtensionName      string
	UpstreamStatusCode int
	UpstreamURL        string
}

// newScreenshotResult creates a ScreenshotResult from an HTTP response.
func newScreenshotResult(resp *http.Response, data []byte) (*ScreenshotResult, error) {
	contentType := resp.Header.Get("Content-Type")
	ext := "bin"
	if parts := strings.Split(contentType, "/"); len(parts) == 2 {
		ext = strings.Split(parts[1], ";")[0]
	}

	statusCodeStr := resp.Header.Get("x-scrapfly-upstream-http-code")
	statusCode, _ := strconv.Atoi(statusCodeStr)

	return &ScreenshotResult{
		Image: data,
		Metadata: ScreenshotMetadata{
			ExtensionName:      ext,
			UpstreamStatusCode: statusCode,
			UpstreamURL:        resp.Header.Get("x-scrapfly-upstream-url"),
		},
	}, nil
}

// ExtractionResult represents the result of an extraction call.
type ExtractionResult struct {
	Data        interface{} `json:"data"`
	ContentType string      `json:"content_type"`
	DataQuality string      `json:"data_quality,omitempty"`
}

// errorResponse is used to unmarshal generic API errors.
type errorResponse struct {
	Message  string `json:"message"`
	ErrorID  string `json:"error_id"`
	HTTPCode int    `json:"http_code"`
	Code     string `json:"code"`
}

// --- Detailed Data Structures ---

// ConfigData mirrors the 'config' object in the API response.
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

// ContextData mirrors the 'context' object in the API response.
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

// ResultData mirrors the 'result' object in the API response.
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
	ExtractedData   interface{}            `json:"extracted_data"`
}

// --- Nested Structures for Context and Result ---

type CacheContext struct {
	State string      `json:"state"`
	Entry interface{} `json:"entry"`
}

type CostDetail struct {
	Amount      int    `json:"amount"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

type CostContext struct {
	Details []CostDetail `json:"details"`
	Total   int          `json:"total"`
}

type DebugContext struct {
	ResponseURL   string      `json:"response_url"`
	ScreenshotURL interface{} `json:"screenshot_url"`
}

type OSContext struct {
	Distribution string `json:"distribution"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Version      string `json:"version"`
}

type ProxyContext struct {
	Country  string `json:"country"`
	Identity string `json:"identity"`
	Network  string `json:"network"`
	Pool     string `json:"pool"`
}

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

type BrowserData struct {
	JSEvaluationResult *string                `json:"javascript_evaluation_result"`
	JSScenario         interface{}            `json:"js_scenario"`
	LocalStorageData   map[string]interface{} `json:"local_storage_data"`
	SessionStorageData map[string]interface{} `json:"session_storage_data"`
	Websockets         []interface{}          `json:"websockets"`
	XHRCall            []interface{}          `json:"xhr_call"`
}

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

type APIErrorDetails struct {
	Code      string            `json:"code"`
	HTTPCode  int               `json:"http_code"`
	Links     map[string]string `json:"links"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	DocURL    string            `json:"doc_url"`
}

type IFrame struct {
	URL     string     `json:"url"`
	URI     URIContext `json:"uri"`
	Content string     `json:"content"`
}

type Screenshot struct {
	CSSSelector *string `json:"css_selector"`
	Extension   string  `json:"extension"`
	Format      string  `json:"format"`
	Size        int     `json:"size"`
	URL         string  `json:"url"`
}

// AccountData represents account information from Scrapfly.
type AccountData struct {
	Account struct {
		AccountID string `json:"account_id"`
		Currency  string `json:"currency"`
		Timezone  string `json:"timezone"`
	} `json:"account"`
	Project struct {
		AllowExtraUsage    bool        `json:"allow_extra_usage"`
		AllowedNetworks    []string    `json:"allowed_networks"`
		BudgetLimit        interface{} `json:"budget_limit"`
		BudgetSpent        interface{} `json:"budget_spent"`
		ConcurrencyLimit   int         `json:"concurrency_limit"`
		Name               string      `json:"name"`
		QuotaReached       bool        `json:"quota_reached"`
		ScrapeRequestCount int         `json:"scrape_request_count"`
		ScrapeRequestLimit int         `json:"scrape_request_limit"`
		Tags               []string    `json:"tags"`
	} `json:"project"`
	Subscription struct {
		Billing struct {
			CurrentExtraScrapeRequestPrice struct {
				Currency string  `json:"currency"`
				Amount   float64 `json:"amount"`
			} `json:"current_extra_scrape_request_price"`
			ExtraScrapeRequestPricePer10k struct {
				Currency string  `json:"currency"`
				Amount   float64 `json:"amount"`
			} `json:"extra_scrape_request_price_per_10k"`
			OngoingPayment struct {
				Currency string  `json:"currency"`
				Amount   float64 `json:"amount"`
			} `json:"ongoing_payment"`
			PlanPrice struct {
				Currency string  `json:"currency"`
				Amount   float64 `json:"amount"`
			} `json:"plan_price"`
		} `json:"billing"`
		ExtraScrapeAllowed bool `json:"extra_scrape_allowed"`
		MaxConcurrency     int  `json:"max_concurrency"`
		Period             struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"period"`
		PlanName string `json:"plan_name"`
		Usage    struct {
			Schedule struct {
				Current int `json:"current"`
				Limit   int `json:"limit"`
			} `json:"schedule"`
			Spider struct {
				Current int `json:"current"`
				Limit   int `json:"limit"`
			} `json:"spider"`
			Scrape struct {
				ConcurrentLimit     int `json:"concurrent_limit"`
				ConcurrentRemaining int `json:"concurrent_remaining"`
				ConcurrentUsage     int `json:"concurrent_usage"`
				Current             int `json:"current"`
				Extra               int `json:"extra"`
				Limit               int `json:"limit"`
				Remaining           int `json:"remaining"`
			} `json:"scrape"`
		} `json:"usage"`
	} `json:"subscription"`
}
