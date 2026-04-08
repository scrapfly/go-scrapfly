package scrapfly

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// HarEntry wraps a single HAR entry (one HTTP request/response pair).
//
// Mirrors the Python SDK's HarEntry class. The accessors live behind getter
// methods (rather than exporting raw maps) so the SDK can normalise the
// `headers: [{name, value}, ...]` array form into a Go-friendly map.
type HarEntry struct {
	data     map[string]interface{}
	request  map[string]interface{}
	response map[string]interface{}
}

// NewHarEntry constructs a HarEntry from a decoded HAR entry map.
// Mostly used internally by HarArchive.GetEntries; exported for tests and
// for callers that already have a parsed HAR map.
func NewHarEntry(data map[string]interface{}) *HarEntry {
	e := &HarEntry{data: data}
	if req, ok := data["request"].(map[string]interface{}); ok {
		e.request = req
	}
	if resp, ok := data["response"].(map[string]interface{}); ok {
		e.response = resp
	}
	return e
}

// URL returns the request URL for this entry.
func (e *HarEntry) URL() string {
	if v, ok := e.request["url"].(string); ok {
		return v
	}
	return ""
}

// Method returns the HTTP method (GET, POST, ...).
func (e *HarEntry) Method() string {
	if v, ok := e.request["method"].(string); ok {
		return v
	}
	return "GET"
}

// StatusCode returns the HTTP response status code, or 0 if missing.
func (e *HarEntry) StatusCode() int {
	if e.response == nil {
		return 0
	}
	switch v := e.response["status"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		// Some HAR producers serialize status as a string. Be tolerant.
		var n int
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}

// StatusText returns the HTTP response status text (e.g. "OK", "Not Found").
func (e *HarEntry) StatusText() string {
	if v, ok := e.response["statusText"].(string); ok {
		return v
	}
	return ""
}

// RequestHeaders returns the request headers as a flat map.
//
// HAR stores headers as `[{name, value}, ...]` to preserve duplicates and
// order; we collapse duplicates by keeping the last occurrence. Use the raw
// data via NewHarEntry's input if you need the full ordered list.
func (e *HarEntry) RequestHeaders() map[string]string {
	return harHeadersAsMap(e.request)
}

// ResponseHeaders returns the response headers as a flat map.
func (e *HarEntry) ResponseHeaders() map[string]string {
	return harHeadersAsMap(e.response)
}

// Content returns the raw response body bytes.
//
// HAR stores response bodies as text inside `content.text`. When the body is
// binary or non-UTF-8, the producer base64-encodes it and sets
// `content.encoding = "base64"`. We honour both forms.
func (e *HarEntry) Content() []byte {
	contentMap, ok := e.response["content"].(map[string]interface{})
	if !ok {
		return nil
	}
	text, _ := contentMap["text"].(string)
	encoding, _ := contentMap["encoding"].(string)
	if encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err == nil {
			return decoded
		}
	}
	return []byte(text)
}

// ContentType returns the response Content-Type (from `response.content.mimeType`).
func (e *HarEntry) ContentType() string {
	contentMap, ok := e.response["content"].(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := contentMap["mimeType"].(string); ok {
		return v
	}
	return ""
}

// ContentSize returns the response body size in bytes (`response.content.size`).
func (e *HarEntry) ContentSize() int {
	contentMap, ok := e.response["content"].(map[string]interface{})
	if !ok {
		return 0
	}
	if v, ok := contentMap["size"].(float64); ok {
		return int(v)
	}
	return 0
}

// StartedDateTime returns the ISO 8601 timestamp when the request started.
func (e *HarEntry) StartedDateTime() string {
	if v, ok := e.data["startedDateTime"].(string); ok {
		return v
	}
	return ""
}

// Time returns the total elapsed time for the request in milliseconds.
func (e *HarEntry) Time() float64 {
	if v, ok := e.data["time"].(float64); ok {
		return v
	}
	return 0
}

// Timings returns the detailed timing breakdown (DNS, blocked, connect, etc.).
func (e *HarEntry) Timings() map[string]float64 {
	timings := make(map[string]float64)
	if t, ok := e.data["timings"].(map[string]interface{}); ok {
		for k, v := range t {
			if f, ok := v.(float64); ok {
				timings[k] = f
			}
		}
	}
	return timings
}

// String makes HarEntry play nicely with fmt.Println.
func (e *HarEntry) String() string {
	return fmt.Sprintf("<HarEntry %s %s [%d]>", e.Method(), e.URL(), e.StatusCode())
}

// harHeadersAsMap normalises a HAR `[{name, value}, ...]` headers array into
// a flat map. Duplicates are collapsed (last occurrence wins).
func harHeadersAsMap(side map[string]interface{}) map[string]string {
	headers := make(map[string]string)
	rawList, ok := side["headers"].([]interface{})
	if !ok {
		return headers
	}
	for _, raw := range rawList {
		header, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := header["name"].(string)
		value, _ := header["value"].(string)
		if name != "" {
			headers[name] = value
		}
	}
	return headers
}

// HarArchive wraps a parsed HAR file with high-level accessors.
//
// Supports both the standard HAR format (single JSON object with `log.entries`)
// and Scrapfly's streaming HAR format (concatenated JSON objects: a leading
// `log` object followed by individual entry objects).
//
// Example:
//
//	artifact, _ := client.CrawlArtifact(uuid, scrapfly.ArtifactTypeHAR)
//	archive, err := scrapfly.ParseHAR(artifact.Data)
//	if err != nil { log.Fatal(err) }
//
//	for _, entry := range archive.Entries() {
//	    fmt.Printf("%s %s [%d]\n", entry.Method(), entry.URL(), entry.StatusCode())
//	}
type HarArchive struct {
	logMap  map[string]interface{}
	entries []map[string]interface{}
}

// ParseHAR decodes a HAR byte stream into a HarArchive. Auto-detects gzip-
// compressed input by checking the magic bytes.
func ParseHAR(data []byte) (*HarArchive, error) {
	// Decompress gzipped input.
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gz.Close()
		decompressed, err := io.ReadAll(gz)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress HAR: %w", err)
		}
		data = decompressed
	}

	// Try to parse as Scrapfly streaming HAR first: concatenated JSON
	// objects separated by whitespace. The first object is the `{"log": ...}`
	// envelope; subsequent objects are individual entries.
	//
	// json.Decoder.More() does this naturally — it can stream multiple
	// values from a single buffer, falling back to standard HAR if the
	// first value contains `log.entries` already populated.
	decoder := json.NewDecoder(bytes.NewReader(data))
	var firstObj map[string]interface{}
	if err := decoder.Decode(&firstObj); err != nil {
		return nil, fmt.Errorf("failed to decode HAR JSON: %w", err)
	}

	logMap, _ := firstObj["log"].(map[string]interface{})
	if logMap == nil {
		return nil, fmt.Errorf("HAR root object missing 'log' field")
	}

	archive := &HarArchive{logMap: logMap}

	// Standard HAR: log.entries is a populated array → use it directly.
	if rawEntries, ok := logMap["entries"].([]interface{}); ok && len(rawEntries) > 0 {
		for _, raw := range rawEntries {
			if entry, ok := raw.(map[string]interface{}); ok {
				archive.entries = append(archive.entries, entry)
			}
		}
	}

	// Scrapfly streaming HAR: any subsequent objects are entries.
	for decoder.More() {
		var entry map[string]interface{}
		if err := decoder.Decode(&entry); err != nil {
			break // tolerate trailing garbage
		}
		archive.entries = append(archive.entries, entry)
	}

	return archive, nil
}

// Version returns the HAR file format version (e.g. "1.2").
func (a *HarArchive) Version() string {
	if v, ok := a.logMap["version"].(string); ok {
		return v
	}
	return ""
}

// Creator returns the creator info block (`{name, version, comment}`).
func (a *HarArchive) Creator() map[string]interface{} {
	if c, ok := a.logMap["creator"].(map[string]interface{}); ok {
		return c
	}
	return map[string]interface{}{}
}

// Pages returns the HAR pages list (each page is a free-form map).
func (a *HarArchive) Pages() []map[string]interface{} {
	rawPages, ok := a.logMap["pages"].([]interface{})
	if !ok {
		return nil
	}
	pages := make([]map[string]interface{}, 0, len(rawPages))
	for _, raw := range rawPages {
		if p, ok := raw.(map[string]interface{}); ok {
			pages = append(pages, p)
		}
	}
	return pages
}

// Entries returns every entry in the archive as a list of HarEntry wrappers.
func (a *HarArchive) Entries() []*HarEntry {
	entries := make([]*HarEntry, 0, len(a.entries))
	for _, raw := range a.entries {
		entries = append(entries, NewHarEntry(raw))
	}
	return entries
}

// IterEntries calls fn for each entry; iteration stops when fn returns false.
func (a *HarArchive) IterEntries(fn func(*HarEntry) bool) {
	for _, raw := range a.entries {
		if !fn(NewHarEntry(raw)) {
			return
		}
	}
}

// URLs returns every distinct URL in the archive in entry order.
func (a *HarArchive) URLs() []string {
	seen := make(map[string]bool)
	var urls []string
	for _, raw := range a.entries {
		entry := NewHarEntry(raw)
		url := entry.URL()
		if url != "" && !seen[url] {
			seen[url] = true
			urls = append(urls, url)
		}
	}
	return urls
}

// FindByURL returns the first entry whose request URL exactly matches `url`,
// or nil if no entry matches.
func (a *HarArchive) FindByURL(url string) *HarEntry {
	for _, raw := range a.entries {
		entry := NewHarEntry(raw)
		if entry.URL() == url {
			return entry
		}
	}
	return nil
}

// FilterByStatus returns every entry whose response has the given HTTP status code.
func (a *HarArchive) FilterByStatus(statusCode int) []*HarEntry {
	var matches []*HarEntry
	for _, raw := range a.entries {
		entry := NewHarEntry(raw)
		if entry.StatusCode() == statusCode {
			matches = append(matches, entry)
		}
	}
	return matches
}

// FilterByContentType returns every entry whose Content-Type contains the
// given substring (case-sensitive). Useful for collecting all HTML responses,
// JavaScript files, images, etc.
func (a *HarArchive) FilterByContentType(contentType string) []*HarEntry {
	var matches []*HarEntry
	for _, raw := range a.entries {
		entry := NewHarEntry(raw)
		if strings.Contains(entry.ContentType(), contentType) {
			matches = append(matches, entry)
		}
	}
	return matches
}

// Len returns the number of entries in the archive.
func (a *HarArchive) Len() int { return len(a.entries) }
