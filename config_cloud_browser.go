package scrapfly

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Valid values for the cloud-browser pool selector. The engine accepts only
// these two literals (per pkg/browser/config.go in scrapfly-api); any other
// value is silently dropped and the default is applied.
const (
	CloudBrowserProxyPoolDatacenter  = "public_datacenter_pool"
	CloudBrowserProxyPoolResidential = "public_residential_pool"
)

// Valid OS values per the engine validation.
const (
	CloudBrowserOSLinux   = "linux"
	CloudBrowserOSWindows = "windows"
	CloudBrowserOSMac     = "mac"
)

// Valid BrowserBrand values per the engine validation. See
// scrapfly-api/pkg/browser/config.go for the authoritative list. Invalid
// values are silently dropped by the server and the default (chrome) applies.
const (
	CloudBrowserBrandChrome = "chrome"
	CloudBrowserBrandEdge   = "edge"
	CloudBrowserBrandBrave  = "brave"
	CloudBrowserBrandOpera  = "opera"
)

// CloudBrowserConfig configures a Cloud Browser session.
//
// All fields are optional. When omitted, the server applies its own
// documented defaults (ProxyPool=public_datacenter_pool, OS=random,
// Country=random from the proxy pool, AutoClose=true, Timeout=900s).
//
// Field names mirror the Cloud Browser API query parameters documented at
// https://scrapfly.io/docs/cloud-browser-api/getting-started — see the
// public docs for the exact behavior of each option.
//
// Example:
//
//	client, _ := scrapfly.New("YOUR_API_KEY")
//	wsURL := client.CloudBrowser(&scrapfly.CloudBrowserConfig{
//	    ProxyPool: scrapfly.CloudBrowserProxyPoolDatacenter,
//	    OS:        scrapfly.CloudBrowserOSLinux,
//	    Country:   "us",
//	})
//	// Pass wsURL to your CDP client (chromedp, playwright-go, etc.)
type CloudBrowserConfig struct {
	// ProxyPool is one of CloudBrowserProxyPoolDatacenter (cheaper, faster)
	// or CloudBrowserProxyPoolResidential (residential IPs for tougher
	// targets). Empty = server picks public_datacenter_pool.
	ProxyPool string

	// OS is the browser OS fingerprint: "linux", "windows", or "mac".
	// Empty = server picks randomly.
	OS string

	// BrowserBrand selects the Chromium-based browser brand for fingerprint
	// generation. Use one of the CloudBrowserBrand* constants. Empty = server
	// uses the default (chrome). Invalid values are silently dropped.
	BrowserBrand string

	// Country is the ISO 3166-1 alpha-2 country code for the proxy exit IP
	// (e.g. "us", "gb", "de"). Empty = server picks from the proxy pool's
	// preferred countries.
	Country string

	// Session is a stable user-supplied session ID. Two sessions with the
	// same ID share the same underlying browser instance.
	Session string

	// AutoClose, when set, controls whether the browser is released as soon
	// as the CDP client disconnects. Use AutoClosePtr() to set false
	// explicitly (since the zero value of *bool is nil = "use server default").
	AutoClose *bool

	// Timeout is the maximum session duration in seconds. Zero = use server
	// default (900s = 15 min). Maximum 1800s.
	Timeout int

	// Resource blocking
	BlockImages *bool
	BlockStyles *bool
	BlockFonts  *bool
	BlockMedia  *bool
	Screenshot  *bool

	// Scrapium features
	Cache     *bool
	Blacklist *bool

	// Extensions is a list of Chrome extension IDs to install in the browser
	// (must be uploaded via the Cloud Browser dashboard first).
	Extensions []string

	// BYOPProxy is a Bring Your Own Proxy URL. Format:
	// {protocol}://{user}:{pass}@{host}:{port}. Supported protocols:
	// http, https, socks5, socks5h, socks5+udp, socks5h+udp.
	// When set, ProxyPool is ignored.
	BYOPProxy string
}

// validate checks the small number of fields where the engine accepts only
// specific literal values. The engine silently drops invalid values, so we
// validate locally to give immediate feedback.
func (c *CloudBrowserConfig) validate() error {
	if c.ProxyPool != "" && c.ProxyPool != CloudBrowserProxyPoolDatacenter && c.ProxyPool != CloudBrowserProxyPoolResidential {
		return fmt.Errorf("CloudBrowserConfig.ProxyPool must be %q or %q, got %q",
			CloudBrowserProxyPoolDatacenter, CloudBrowserProxyPoolResidential, c.ProxyPool)
	}
	if c.OS != "" && c.OS != CloudBrowserOSLinux && c.OS != CloudBrowserOSWindows && c.OS != CloudBrowserOSMac {
		return fmt.Errorf("CloudBrowserConfig.OS must be one of 'linux'/'windows'/'mac', got %q", c.OS)
	}
	if c.BrowserBrand != "" &&
		c.BrowserBrand != CloudBrowserBrandChrome &&
		c.BrowserBrand != CloudBrowserBrandEdge &&
		c.BrowserBrand != CloudBrowserBrandBrave &&
		c.BrowserBrand != CloudBrowserBrandOpera {
		return fmt.Errorf("CloudBrowserConfig.BrowserBrand must be one of 'chrome'/'edge'/'brave'/'opera', got %q", c.BrowserBrand)
	}
	return nil
}

// toQueryParams serializes this config to URL values, dropping unset fields
// so the server applies its own defaults.
func (c *CloudBrowserConfig) toQueryParams() url.Values {
	params := url.Values{}
	if c.ProxyPool != "" {
		params.Set("proxy_pool", c.ProxyPool)
	}
	if c.OS != "" {
		params.Set("os", c.OS)
	}
	if c.BrowserBrand != "" {
		params.Set("browser_brand", c.BrowserBrand)
	}
	if c.Country != "" {
		params.Set("country", c.Country)
	}
	if c.Session != "" {
		params.Set("session", c.Session)
	}
	if c.BYOPProxy != "" {
		params.Set("byop_proxy", c.BYOPProxy)
	}
	if c.Timeout > 0 {
		params.Set("timeout", strconv.Itoa(c.Timeout))
	}
	setBool := func(name string, ptr *bool) {
		if ptr != nil {
			if *ptr {
				params.Set(name, "true")
			} else {
				params.Set(name, "false")
			}
		}
	}
	setBool("auto_close", c.AutoClose)
	setBool("block_images", c.BlockImages)
	setBool("block_styles", c.BlockStyles)
	setBool("block_fonts", c.BlockFonts)
	setBool("block_media", c.BlockMedia)
	setBool("screenshot", c.Screenshot)
	setBool("cache", c.Cache)
	setBool("blacklist", c.Blacklist)
	if len(c.Extensions) > 0 {
		params.Set("extensions", strings.Join(c.Extensions, ","))
	}
	return params
}

// CloudBrowserBoolPtr returns a pointer to the given bool. Convenience helper
// for the *bool fields on CloudBrowserConfig (e.g. AutoClose, BlockImages).
//
//	cfg := &scrapfly.CloudBrowserConfig{
//	    AutoClose:   scrapfly.CloudBrowserBoolPtr(false),
//	    BlockImages: scrapfly.CloudBrowserBoolPtr(true),
//	}
func CloudBrowserBoolPtr(b bool) *bool {
	return &b
}
