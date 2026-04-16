package scrapfly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultCloudBrowserHost = "https://browser.scrapfly.io"
)

// CloudBrowserConfig configures a Cloud Browser session.
type CloudBrowserConfig struct {
	ProxyPool    string   `json:"proxy_pool,omitempty"`
	OS           string   `json:"os,omitempty"`
	Country      string   `json:"country,omitempty"`
	Session      string   `json:"session,omitempty"`
	Timeout      int      `json:"timeout,omitempty"` // Session timeout in seconds (default 900)
	BlockImages  bool     `json:"block_images,omitempty"`
	BlockStyles  bool     `json:"block_styles,omitempty"`
	BlockFonts   bool     `json:"block_fonts,omitempty"`
	BlockMedia   bool     `json:"block_media,omitempty"`
	Screenshot   bool     `json:"screenshot,omitempty"`
	Cache        bool     `json:"cache,omitempty"`
	Blacklist    bool     `json:"blacklist,omitempty"`
	Debug        bool     `json:"debug,omitempty"`
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

	// EnableMCP enables Chromium's built-in Model Context Protocol (MCP) support.
	// When true, the browser exposes a streamable-HTTP MCP endpoint for AI agents.
	EnableMCP bool `json:"enable_mcp,omitempty"`
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
		if config.EnableMCP {
			params.Set("enable_mcp", "true")
		}
	}

	// Normalize `host` to a wss:// URL regardless of the scheme the caller
	// configured. Accepted input schemes: https:// (default), wss://, ws://,
	// http:// (dev stacks). Dev stacks on .home use ws:// since they don't
	// carry a real TLS cert on the browser service.
	hostNoScheme := host
	var wsScheme string
	switch {
	case strings.HasPrefix(host, "wss://"):
		hostNoScheme = strings.TrimPrefix(host, "wss://")
		wsScheme = "wss"
	case strings.HasPrefix(host, "ws://"):
		hostNoScheme = strings.TrimPrefix(host, "ws://")
		wsScheme = "ws"
	case strings.HasPrefix(host, "https://"):
		hostNoScheme = strings.TrimPrefix(host, "https://")
		wsScheme = "wss"
	case strings.HasPrefix(host, "http://"):
		hostNoScheme = strings.TrimPrefix(host, "http://")
		wsScheme = "ws"
	default:
		wsScheme = "wss"
	}
	return fmt.Sprintf("%s://%s?%s", wsScheme, hostNoScheme, params.Encode())
}

// UnblockConfig configures an unblock request.
type UnblockConfig struct {
	URL            string `json:"url"`
	Country        string `json:"country,omitempty"`
	Timeout        int    `json:"timeout,omitempty"`         // Navigation timeout in seconds
	BrowserTimeout int    `json:"browser_timeout,omitempty"` // Browser session timeout in seconds
	EnableMCP      bool   `json:"enable_mcp,omitempty"`      // Enable MCP support in the browser
}

// UnblockResult is the response from the /unblock endpoint.
type UnblockResult struct {
	WSURL       string `json:"ws_url"`
	SessionID   string `json:"session_id"`
	RunID       string `json:"run_id"`
	MCPEndpoint string `json:"mcp_endpoint,omitempty"` // MCP streamable-HTTP endpoint (only when EnableMCP=true)
}

// cloudBrowserRESTHost returns the configured cloud browser host normalized to
// an https:// / http:// form suitable for REST calls. Callers typically
// configure a `wss://` / `ws://` host (the CDP websocket entry point); the
// REST endpoints (`/unblock`, `/session/.../stop`, `/extension`) live on the
// same host under the matching http scheme.
func (c *Client) cloudBrowserRESTHost() string {
	host := c.cloudBrowserHost
	if host == "" {
		host = defaultCloudBrowserHost
	}
	switch {
	case strings.HasPrefix(host, "wss://"):
		return "https://" + strings.TrimPrefix(host, "wss://")
	case strings.HasPrefix(host, "ws://"):
		return "http://" + strings.TrimPrefix(host, "ws://")
	default:
		return host
	}
}

// CloudBrowserUnblock bypasses anti-bot protection and returns a WebSocket URL
// to a browser with cookies/state pre-loaded.
func (c *Client) CloudBrowserUnblock(config UnblockConfig) (*UnblockResult, error) {
	host := c.cloudBrowserRESTHost()
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
	host := c.cloudBrowserRESTHost()
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stop failed with status %d", resp.StatusCode)
	}

	return nil
}

// CloudBrowserPlayback returns debug recording metadata for a given run ID.
func (c *Client) CloudBrowserPlayback(runID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/run/%s/playback?key=%s", host, url.PathEscape(runID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create playback request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("playback request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playback failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode playback response: %w", err)
	}
	return result, nil
}

// CloudBrowserVideo downloads a debug session recording video.
// Returns the raw video bytes (webm format).
func (c *Client) CloudBrowserVideo(runID string) ([]byte, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/run/%s/video?key=%s", host, url.PathEscape(runID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create video request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("video download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// CloudBrowserSessions lists all running Cloud Browser sessions.
func (c *Client) CloudBrowserSessions() (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/sessions?key=%s", host, url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sessions request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sessions request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sessions list failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode sessions response: %w", err)
	}
	return result, nil
}

// CloudBrowserExtensionList lists all browser extensions for the account.
func (c *Client) CloudBrowserExtensionList() (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/extension?key=%s", host, url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create extension list request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extension list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extension list failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode extension list response: %w", err)
	}
	return result, nil
}

// CloudBrowserExtensionGet returns details of a specific extension.
func (c *Client) CloudBrowserExtensionGet(extensionID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/extension/%s?key=%s", host, url.PathEscape(extensionID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create extension get request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extension get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extension get failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode extension get response: %w", err)
	}
	return result, nil
}

// CloudBrowserExtensionUpload uploads a browser extension from a local .zip or .crx file.
func (c *Client) CloudBrowserExtensionUpload(filePath string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/extension?key=%s", host, url.QueryEscape(c.key))

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open extension file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest(http.MethodPost, reqURL, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extension upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}
	return result, nil
}

// CloudBrowserExtensionDelete deletes a browser extension by ID.
func (c *Client) CloudBrowserExtensionDelete(extensionID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/extension/%s?key=%s", host, url.PathEscape(extensionID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create extension delete request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extension delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("extension delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode delete response: %w", err)
	}
	return result, nil
}
