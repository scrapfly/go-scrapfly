package scrapfly

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultCloudBrowserHost = "https://browser.scrapfly.io"
)

// CloudBrowserConfig configures a Cloud Browser session.
type CloudBrowserConfig struct {
	ProxyPool   string   `json:"proxy_pool,omitempty"`
	OS          string   `json:"os,omitempty"`
	Country     string   `json:"country,omitempty"`
	Session     string   `json:"session,omitempty"`
	Timeout     int      `json:"timeout,omitempty"`      // Session timeout in seconds (default 900)
	BlockImages bool     `json:"block_images,omitempty"`
	BlockStyles bool     `json:"block_styles,omitempty"`
	BlockFonts  bool     `json:"block_fonts,omitempty"`
	BlockMedia  bool     `json:"block_media,omitempty"`
	Screenshot  bool     `json:"screenshot,omitempty"`
	Cache       bool     `json:"cache,omitempty"`
	Blacklist   bool     `json:"blacklist,omitempty"`
	Debug       bool     `json:"debug,omitempty"`
	Resolution   string   `json:"resolution,omitempty"`
	Extensions   []string `json:"extensions,omitempty"`
	BrowserBrand string   `json:"browser_brand,omitempty"`

	// BYOPProxy is a "Bring Your Own Proxy" URL that the Cloud Browser will use
	// instead of Scrapfly's managed proxy pools.
	//
	// Format: {protocol}://{user}:{pass}@{host}:{port}
	// Supported protocols: http, https, socks5, socks5h, socks5+udp, socks5h+udp.
	// The +udp variants enable HTTP/3 (QUIC) via SOCKS5 UDP ASSOCIATE — only
	// works with proxy providers that implement RFC 1928 UDP ASSOCIATE.
	//
	// Requires a Custom plan subscription.
	// See https://scrapfly.io/docs/cloud-browser-api/byop for details.
	BYOPProxy string `json:"byop_proxy,omitempty"`
}

// WebSocketURL returns the Cloud Browser WebSocket connection URL.
func (c *Client) CloudBrowser(config *CloudBrowserConfig) string {
	host := c.cloudBrowserHost
	if host == "" {
		host = defaultCloudBrowserHost
	}

	params := url.Values{}
	params.Set("api_key", c.key)

	if config != nil {
		if config.ProxyPool != "" {
			params.Set("proxy_pool", config.ProxyPool)
		}
		if config.OS != "" {
			params.Set("os", config.OS)
		}
		if config.Country != "" {
			params.Set("country", config.Country)
		}
		if config.Session != "" {
			params.Set("session", config.Session)
		}
		if config.Timeout > 0 {
			params.Set("timeout", fmt.Sprintf("%d", config.Timeout))
		}
		if config.BlockImages {
			params.Set("block_images", "true")
		}
		if config.BlockStyles {
			params.Set("block_styles", "true")
		}
		if config.BlockFonts {
			params.Set("block_fonts", "true")
		}
		if config.BlockMedia {
			params.Set("block_media", "true")
		}
		if config.Screenshot {
			params.Set("screenshot", "true")
		}
		if config.Cache {
			params.Set("cache", "true")
		}
		if config.Blacklist {
			params.Set("blacklist", "true")
		}
		if config.Debug {
			params.Set("debug", "true")
		}
		if config.Resolution != "" {
			params.Set("resolution", config.Resolution)
		}
		if config.BrowserBrand != "" {
			params.Set("browser_brand", config.BrowserBrand)
		}
		if config.BYOPProxy != "" {
			params.Set("byop_proxy", config.BYOPProxy)
		}
	}

	return fmt.Sprintf("wss://%s?%s", strings.TrimPrefix(host, "https://"), params.Encode())
}

// UnblockConfig configures an unblock request.
type UnblockConfig struct {
	URL            string `json:"url"`
	Country        string `json:"country,omitempty"`
	Timeout        int    `json:"timeout,omitempty"`        // Navigation timeout in seconds
	BrowserTimeout int    `json:"browser_timeout,omitempty"` // Browser session timeout in seconds
}

// UnblockResult is the response from the /unblock endpoint.
type UnblockResult struct {
	WSURL     string `json:"ws_url"`
	SessionID string `json:"session_id"`
	RunID     string `json:"run_id"`
}

// CloudBrowserUnblock bypasses anti-bot protection and returns a WebSocket URL
// to a browser with cookies/state pre-loaded.
func (c *Client) CloudBrowserUnblock(config UnblockConfig) (*UnblockResult, error) {
	host := c.cloudBrowserHost
	if host == "" {
		host = defaultCloudBrowserHost
	}

	reqURL := fmt.Sprintf("%s/unblock?key=%s", host, url.QueryEscape(c.key))

	body, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unblock config: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create unblock request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unblock request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read unblock response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unblock failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result UnblockResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse unblock response: %w", err)
	}

	return &result, nil
}

// CloudBrowserSessionStop terminates a browser session.
func (c *Client) CloudBrowserSessionStop(sessionID string) error {
	host := c.cloudBrowserHost
	if host == "" {
		host = defaultCloudBrowserHost
	}

	reqURL := fmt.Sprintf("%s/session/%s/stop?key=%s", host, url.PathEscape(sessionID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodPost, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create stop request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("stop request failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stop failed with status %d", resp.StatusCode)
	}

	return nil
}
