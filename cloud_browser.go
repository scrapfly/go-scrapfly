package scrapfly

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	// EnableMCP enables Scrapium's built-in Model Context Protocol (MCP) support.
	// When true, the browser exposes a streamable-HTTP MCP endpoint for AI agents.
	EnableMCP bool `json:"enable_mcp,omitempty"`

	// SolveCaptcha arms Scrapium's built-in captcha detector + solver on the
	// first page attach. Turnstile, DataDome slider, reCAPTCHA, GeeTest,
	// PerimeterX hold, and puzzle-click captchas are handled automatically —
	// no extra CDP calls from the client.
	//
	// Billed per solve (5 credits for interaction-based, 20 for token-based);
	// a failed attempt (captchaError) costs nothing.
	// See https://scrapfly.io/docs/cloud-browser-api/captcha-solver for details.
	SolveCaptcha bool `json:"solve_captcha,omitempty"`

	// Vault is the credential vault NAME to attach to the session — the
	// alphanumeric name you gave it at create time. The API resolves it to
	// the vault scoped to your api-key's project and environment, then
	// injects its credentials (passwords, passkeys, cookies, TOTP codes)
	// into matching origins via CDP. Pair with VaultKey — the customer-held
	// base64 32-byte key the API needs to decrypt the vault for this session
	// only. Both land on the wss:// URL as query parameters; the API never
	// persists VaultKey.
	//
	// Treat VaultKey as secret material — never log it.
	Vault    string `json:"vault,omitempty"`
	VaultKey string `json:"vault_key,omitempty"`

	// EnableVNC turns on the human-in-the-loop VNC channel (operator attach
	// via Scrapfly's VNC mux at :5901).
	EnableVNC bool `json:"enable_vnc,omitempty"`

	// VNCPassword is the customer-chosen VNC password. Required when
	// EnableVNC is true and HITLAllowedNetworks is empty.
	VNCPassword string `json:"vnc_password,omitempty"`

	// EnableRTC turns on the human-in-the-loop WebRTC channel.
	EnableRTC bool `json:"enable_rtc,omitempty"`

	// RTCUsername is the WebRTC username. Defaults to "scrapfly" server-side.
	RTCUsername string `json:"rtc_username,omitempty"`

	// RTCPassword is the customer-chosen WebRTC password. Required when
	// EnableRTC is true and HITLAllowedNetworks is empty.
	RTCPassword string `json:"rtc_password,omitempty"`

	// HITLAllowedNetworks lists source IPs / CIDRs trusted to attach to
	// the HITL channels (VNC + WebRTC + downloads) without credentials.
	HITLAllowedNetworks []string `json:"hitl_allowed_networks,omitempty"`
}

// ProjectSalt returns the deterministic project salt for an api key
// (sha256(apiKey)[:8]). Matches the X-Browser-Project-Salt response
// header returned on a successful Cloud Browser WebSocket upgrade.
func ProjectSalt(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])[:8]
}

// CloudBrowserProjectSalt returns the project salt for this client's api key.
func (c *Client) CloudBrowserProjectSalt() string {
	return ProjectSalt(c.key)
}

// CloudBrowser returns the Cloud Browser WebSocket connection URL.
//
// On rejection the server sends a JSON error frame then a close frame
// with code 1008/1011/1013 and a "ERR::BROWSER::CODE: reason" string.
// See https://scrapfly.io/docs/cloud-browser-api/errors#websocket-close-frame
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
		if config.SolveCaptcha {
			params.Set("solve_captcha", "true")
		}
		if config.Vault != "" {
			params.Set("vault", config.Vault)
		}
		if config.VaultKey != "" {
			// VaultKey is sensitive material; it is forwarded on the
			// wss:// URL exactly as supplied and is never logged or
			// persisted by the SDK.
			params.Set("vault_key", config.VaultKey)
		}
		if config.EnableVNC {
			params.Set("enable_vnc", "true")
		}
		if config.VNCPassword != "" {
			params.Set("vnc_password", config.VNCPassword)
		}
		if config.EnableRTC {
			params.Set("enable_rtc", "true")
		}
		if config.RTCUsername != "" {
			params.Set("rtc_username", config.RTCUsername)
		}
		if config.RTCPassword != "" {
			params.Set("rtc_password", config.RTCPassword)
		}
		if len(config.HITLAllowedNetworks) > 0 {
			params.Set("hitl_allowed_networks", strings.Join(config.HITLAllowedNetworks, ","))
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
	OS             string `json:"os,omitempty"`              // Fingerprint OS: linux, windows, macos
	BrowserBrand   string `json:"browser_brand,omitempty"`   // Fingerprint browser brand: chrome, edge, brave, opera
	Timeout        int    `json:"timeout,omitempty"`         // Navigation timeout in seconds
	BrowserTimeout int    `json:"browser_timeout,omitempty"` // Browser session timeout in seconds
	EnableMCP      bool   `json:"enable_mcp,omitempty"`      // Enable MCP support in the browser
	Debug          bool   `json:"debug,omitempty"`           // Record the session for replay via CloudBrowserPlayback / CloudBrowserVideo
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
// The response carries `available`, `status` (one of `ready`, `uploading`,
// `unavailable`, `disabled`), `metadata`, `video_url`, and `retry_after_ms`.
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

// CloudBrowserWaitForPlayback polls the playback endpoint until the recording
// resolves to a terminal state (`ready` or `unavailable`) or the timeout
// elapses. It honours the server's `retry_after_ms` hint when present, and
// falls back to fallbackInterval otherwise. Pass timeout == 0 to fall back to
// 3 minutes; fallbackInterval == 0 falls back to 3 seconds.
func (c *Client) CloudBrowserWaitForPlayback(runID string, timeout, fallbackInterval time.Duration) (map[string]interface{}, error) {
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	if fallbackInterval <= 0 {
		fallbackInterval = 3 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		playback, err := c.CloudBrowserPlayback(runID)
		if err != nil {
			return nil, err
		}
		status, _ := playback["status"].(string)
		if status != "uploading" {
			return playback, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return playback, nil
		}
		var sleep time.Duration
		if raw, ok := playback["retry_after_ms"]; ok {
			if ms, ok := raw.(float64); ok && ms > 0 {
				sleep = time.Duration(ms) * time.Millisecond
			}
		}
		if sleep <= 0 {
			sleep = fallbackInterval
		}
		if sleep > remaining {
			sleep = remaining
		}
		time.Sleep(sleep)
	}
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

// ----------------------------------------------------------------------
// Cloud Browser Credential Vault — CRUD
//
// The vault stores per-origin credentials (passwords, passkeys, cookies,
// TOTP seeds) that the Cloud Browser injects via CDP at session attach
// time. Every vault is end-to-end encrypted under a customer-held key
// generated on POST /vault and returned exactly once. The API never
// persists the key — clients must store it locally.
//
// Security contract for the SDK:
//
//   - The vault key (`vaultKey`, `currentVaultKey`) MUST NEVER appear in
//     log output, error messages, panic traces, or breadcrumbs. The
//     wrappers below pass it straight to the X-Vault-Key header without
//     interpolation. If a request fails, only the response status and
//     server-supplied message are surfaced — the header value is not
//     reflected.
//   - Server-encrypted item blobs are returned by GET /vault/{id}/item
//     (no plaintext); they are useless without the customer-held key.
//
// The `vaultErrorf` helper centralises error formatting and exists so
// that any future "include request context" patches stay key-free.
// ----------------------------------------------------------------------

// vaultErrorf formats a vault REST error without ever embedding the
// caller's vault key. It accepts the operation label, the HTTP status,
// and the server-supplied response body — never the header value.
func vaultErrorf(op string, status int, body []byte) error {
	return fmt.Errorf("vault %s failed with status %d: %s", op, status, string(body))
}

// CloudBrowserVaultCreate creates a new credential vault.
//
// The response includes a freshly minted base64 32-byte vault key under
// the `key` field. The server emits this exactly once — callers MUST
// store it locally. It is required for any subsequent item read/write
// or for vault rotation.
func (c *Client) CloudBrowserVaultCreate(name, description string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault?key=%s", host, url.QueryEscape(c.key))

	body, err := json.Marshal(map[string]string{"name": name, "description": description})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vault create body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create vault create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault create request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("create", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault create response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultList returns every vault visible to the API key in
// the current project + environment. Response shape: {vaults: [...]}.
func (c *Client) CloudBrowserVaultList() (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault?key=%s", host, url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault list request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("list", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault list response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultGet returns the metadata for one vault.
// No secret material is included in the response.
func (c *Client) CloudBrowserVaultGet(vaultID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s?key=%s", host, url.PathEscape(vaultID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault get request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("get", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault get response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultUpdate patches a vault's metadata. Empty-string args
// are treated as "no change" and omitted from the wire payload — pass
// only the fields you want to overwrite.
func (c *Client) CloudBrowserVaultUpdate(vaultID string, name, description string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s?key=%s", host, url.PathEscape(vaultID), url.QueryEscape(c.key))

	patch := map[string]string{}
	if name != "" {
		patch["name"] = name
	}
	if description != "" {
		patch["description"] = description
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vault update body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create vault update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault update request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("update", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault update response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultDelete removes a vault and every item it holds.
// Cannot be reversed.
func (c *Client) CloudBrowserVaultDelete(vaultID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s?key=%s", host, url.PathEscape(vaultID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault delete request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("delete", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault delete response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultRotate rotates the vault key. The OLD key MUST be
// supplied as currentVaultKey — the API uses it to decrypt every item
// and rewraps each per-row DEK under a freshly generated key, returned
// exactly once in the `key` field of the response. After this call the
// old key cannot read any item in the vault.
//
// currentVaultKey is forwarded as the X-Vault-Key header. It is NEVER
// logged, included in error messages, or echoed back via Recorder
// breadcrumbs — see the security contract at the top of this section.
func (c *Client) CloudBrowserVaultRotate(vaultID, currentVaultKey string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s/rotate?key=%s", host, url.PathEscape(vaultID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodPost, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault rotate request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("X-Vault-Key", currentVaultKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault rotate request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("rotate", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault rotate response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultItemList returns every item in a vault.
//
// The server returns metadata + the envelope-encrypted secret_blob —
// plaintext secrets are not exposed by this endpoint and never reach
// the SDK. To read plaintext, the dashboard performs WebCrypto on the
// blob locally with the customer-held vault key.
func (c *Client) CloudBrowserVaultItemList(vaultID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s/item?key=%s", host, url.PathEscape(vaultID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault item list request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault item list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("item list", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault item list response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultItemCreate adds a credential to a vault. The item
// payload follows the controller's typed shape:
//
//	map[string]interface{}{
//	    "type":   "password",
//	    "label":  "web-scraping.dev test",
//	    "origin": "https://web-scraping.dev/password-manager-test",
//	    "username": "user123",
//	    "secret": map[string]interface{}{"password": "password"},
//	}
//
// vaultKey is forwarded as the X-Vault-Key header. Treat it as secret
// material — the SDK will not log it.
func (c *Client) CloudBrowserVaultItemCreate(vaultID, vaultKey string, item map[string]interface{}) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s/item?key=%s", host, url.PathEscape(vaultID), url.QueryEscape(c.key))

	body, err := json.Marshal(item)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vault item body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create vault item create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("X-Vault-Key", vaultKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault item create request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("item create", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault item create response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultItemUpdate patches a vault item.
//
// Pass vaultKey only when the patch includes secret rotation — i.e. a
// non-nil "secret" key in `patch`. For metadata-only patches (label,
// origin, username), pass an empty string and the X-Vault-Key header
// is not sent.
func (c *Client) CloudBrowserVaultItemUpdate(vaultID, itemID, vaultKey string, patch map[string]interface{}) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s/item/%s?key=%s",
		host, url.PathEscape(vaultID), url.PathEscape(itemID), url.QueryEscape(c.key))

	body, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vault item patch: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create vault item update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", sdkUserAgent)
	if vaultKey != "" {
		req.Header.Set("X-Vault-Key", vaultKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault item update request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("item update", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault item update response: %w", err)
	}
	return result, nil
}

// CloudBrowserVaultItemDelete removes a single item from a vault.
// No vault key is required — the row is dropped without decryption.
func (c *Client) CloudBrowserVaultItemDelete(vaultID, itemID string) (map[string]interface{}, error) {
	host := c.cloudBrowserRESTHost()
	reqURL := fmt.Sprintf("%s/vault/%s/item/%s?key=%s",
		host, url.PathEscape(vaultID), url.PathEscape(itemID), url.QueryEscape(c.key))

	req, err := http.NewRequest(http.MethodDelete, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault item delete request: %w", err)
	}
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault item delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, vaultErrorf("item delete", resp.StatusCode, respBody)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode vault item delete response: %w", err)
	}
	return result, nil
}
