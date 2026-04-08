package scrapfly

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// WarcRecord represents a single WARC record parsed from a crawl artifact.
//
// Mirrors the dataclass exposed by the Python SDK's `WarcRecord`.
type WarcRecord struct {
	// RecordType is the value of the WARC-Type header (e.g. "response", "request",
	// "metadata", "warcinfo").
	RecordType string

	// URL is the value of the WARC-Target-URI header.
	URL string

	// Headers contains the parsed HTTP response headers (only populated for
	// "response" records). Header names are kept in their original casing.
	Headers map[string]string

	// Content is the raw record body. For "response" records, this is the HTTP
	// response body (after the headers section); for other types, this is the
	// raw content block as it appeared in the WARC.
	Content []byte

	// StatusCode is the parsed HTTP status code for "response" records.
	// Zero for non-response records or when the status line couldn't be parsed.
	StatusCode int

	// WARCHeaders contains all the WARC-level headers (WARC-Type, WARC-Date, etc.).
	WARCHeaders map[string]string
}

// WarcParser walks a WARC byte stream record-by-record.
//
// The parser handles both gzip-compressed and uncompressed WARC data and
// supports the subset of the WARC 1.0 format that the Scrapfly Crawler
// produces in its artifacts. It is intentionally minimal — for general-purpose
// WARC consumption, prefer a dedicated library like github.com/nlnwa/gowarc.
//
// Example:
//
//	artifact, _ := client.CrawlArtifact(uuid, scrapfly.ArtifactTypeWARC)
//	parser, err := scrapfly.ParseWARC(artifact.Data)
//	if err != nil { log.Fatal(err) }
//
//	pages, _ := parser.GetPages()
//	for _, page := range pages {
//	    fmt.Printf("%s: %d bytes\n", page.URL, len(page.Content))
//	}
type WarcParser struct {
	data []byte
}

// httpStatusLineRE matches "HTTP/1.0 200 OK" / "HTTP/1.1 404 Not Found" / etc.
var httpStatusLineRE = regexp.MustCompile(`^HTTP/\d\.\d (\d+)`)

// ParseWARC creates a WarcParser from raw WARC bytes.
//
// Auto-detects gzip-compressed input by checking the magic bytes (1f 8b).
// Returns a non-nil error only when the gzip header is present but the
// payload fails to decompress.
func ParseWARC(data []byte) (*WarcParser, error) {
	// Detect and decompress gzip-wrapped WARCs (the standard WARC distribution format).
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gz.Close()
		decompressed, err := io.ReadAll(gz)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress WARC: %w", err)
		}
		data = decompressed
	}
	return &WarcParser{data: data}, nil
}

// IterRecords walks every record in the WARC stream and invokes fn for each
// successfully parsed record. Iteration stops when fn returns false (treat
// the bool as "continue", not "ok").
//
// Returns the number of records yielded and any error encountered while
// reading the underlying byte stream.
func (p *WarcParser) IterRecords(fn func(*WarcRecord) bool) (int, error) {
	reader := bufio.NewReader(bytes.NewReader(p.data))
	count := 0
	for {
		// Read the WARC version line ("WARC/1.0\r\n").
		versionLine, err := readLine(reader)
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return count, fmt.Errorf("failed to read WARC version line: %w", err)
		}
		if !strings.HasPrefix(versionLine, "WARC/") {
			// Not a WARC version marker — either we hit padding between
			// records or the input is malformed. Either way, stop.
			return count, nil
		}

		// Read the WARC headers up to the blank line.
		warcHeaders, err := readHeaders(reader)
		if err != nil {
			return count, fmt.Errorf("failed to read WARC headers: %w", err)
		}

		contentLength := 0
		if v, ok := warcHeaders["Content-Length"]; ok {
			if n, parseErr := strconv.Atoi(strings.TrimSpace(v)); parseErr == nil {
				contentLength = n
			}
		}

		// Read exactly Content-Length bytes of content block.
		contentBlock := make([]byte, contentLength)
		if contentLength > 0 {
			if _, err := io.ReadFull(reader, contentBlock); err != nil {
				return count, fmt.Errorf("failed to read WARC content block: %w", err)
			}
		}

		// Skip the two trailing blank lines that separate records.
		_, _ = readLine(reader)
		_, _ = readLine(reader)

		record := parseWarcRecord(warcHeaders, contentBlock)
		if record != nil {
			count++
			if !fn(record) {
				return count, nil
			}
		}
	}
}

// IterResponses is a convenience helper that yields only HTTP response records
// (record_type=response) with a non-zero status code.
func (p *WarcParser) IterResponses(fn func(*WarcRecord) bool) (int, error) {
	count := 0
	_, err := p.IterRecords(func(r *WarcRecord) bool {
		if r.RecordType != "response" || r.StatusCode == 0 {
			return true
		}
		count++
		return fn(r)
	})
	return count, err
}

// WarcPage is a simplified view of a single crawled page extracted from a
// WARC artifact. Use it when you don't need the raw record / WARC headers.
type WarcPage struct {
	URL        string
	StatusCode int
	Headers    map[string]string
	Content    []byte
}

// GetPages returns every HTTP response in the WARC as a slim WarcPage.
//
// This is the easiest way to walk a crawler's artifact without dealing with
// the WARC format directly. For very large artifacts, use IterResponses
// instead so you can stream records one at a time.
func (p *WarcParser) GetPages() ([]WarcPage, error) {
	var pages []WarcPage
	_, err := p.IterResponses(func(r *WarcRecord) bool {
		pages = append(pages, WarcPage{
			URL:        r.URL,
			StatusCode: r.StatusCode,
			Headers:    r.Headers,
			Content:    r.Content,
		})
		return true
	})
	return pages, err
}

// readLine reads up to and including the next \n, then strips trailing \r\n.
// Used to parse WARC version lines and header rows.
func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readHeaders reads `Key: Value` rows until a blank line, returning a map.
// Used by both the WARC-record header section and the inner HTTP headers.
func readHeaders(r *bufio.Reader) (map[string]string, error) {
	headers := make(map[string]string)
	for {
		line, err := readLine(r)
		if err == io.EOF && line == "" {
			return headers, nil
		}
		if err != nil && line == "" {
			return headers, err
		}
		if line == "" {
			return headers, nil
		}
		colon := strings.Index(line, ":")
		if colon == -1 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		headers[key] = value
	}
}

// parseWarcRecord turns the raw WARC headers + content block into a typed
// WarcRecord. Returns nil for unknown record types.
func parseWarcRecord(warcHeaders map[string]string, content []byte) *WarcRecord {
	recordType := warcHeaders["WARC-Type"]
	url := warcHeaders["WARC-Target-URI"]

	switch recordType {
	case "response":
		httpHeaders, body := parseHTTPResponse(content)
		statusCode := extractHTTPStatusCode(content)
		return &WarcRecord{
			RecordType:  recordType,
			URL:         url,
			Headers:     httpHeaders,
			Content:     body,
			StatusCode:  statusCode,
			WARCHeaders: warcHeaders,
		}
	case "request", "metadata", "warcinfo":
		return &WarcRecord{
			RecordType:  recordType,
			URL:         url,
			Headers:     map[string]string{},
			Content:     content,
			WARCHeaders: warcHeaders,
		}
	}
	return nil
}

// parseHTTPResponse splits a WARC `response` content block into the parsed
// HTTP headers and the response body.
//
// The block is in HTTP-over-the-wire format: status line, headers, blank
// line, body. We split on the first `\r\n\r\n` (or `\n\n` for stragglers).
func parseHTTPResponse(content []byte) (map[string]string, []byte) {
	headers := make(map[string]string)

	sep := []byte("\r\n\r\n")
	idx := bytes.Index(content, sep)
	if idx == -1 {
		sep = []byte("\n\n")
		idx = bytes.Index(content, sep)
	}
	if idx == -1 {
		// No header/body separator — return everything as body.
		return headers, content
	}
	headerSection := content[:idx]
	body := content[idx+len(sep):]

	// Skip the status line and parse the rest as `Key: Value`.
	lines := bytes.Split(headerSection, []byte("\n"))
	for i, line := range lines {
		if i == 0 {
			continue // status line
		}
		line = bytes.TrimRight(line, "\r")
		colon := bytes.Index(line, []byte(":"))
		if colon == -1 {
			continue
		}
		key := strings.TrimSpace(string(line[:colon]))
		value := strings.TrimSpace(string(line[colon+1:]))
		headers[key] = value
	}
	return headers, body
}

// extractHTTPStatusCode pulls the numeric status code from the first line of
// an HTTP response (e.g. "HTTP/1.1 200 OK" → 200).
func extractHTTPStatusCode(content []byte) int {
	// Get the first line, terminated by \r\n or \n.
	end := bytes.IndexAny(content, "\r\n")
	if end == -1 {
		return 0
	}
	firstLine := content[:end]
	matches := httpStatusLineRE.FindSubmatch(firstLine)
	if len(matches) < 2 {
		return 0
	}
	code, err := strconv.Atoi(string(matches[1]))
	if err != nil {
		return 0
	}
	return code
}
