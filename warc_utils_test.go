package scrapfly

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// makeWARC builds a small in-memory WARC file with a single HTTP response
// record. The WARC 1.0 format is straightforward enough to construct by hand
// for tests — using a real WARC library here would just create a circular
// dependency.
func makeWARC() []byte {
	// HTTP response block (status line + headers + blank + body)
	httpResponse := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Length: 13\r\n" +
		"\r\n" +
		"<html></html>"

	// WARC record envelope
	warcHeaders := "WARC/1.0\r\n" +
		"WARC-Type: response\r\n" +
		"WARC-Target-URI: https://example.com/page\r\n" +
		"WARC-Date: 2026-01-01T00:00:00Z\r\n" +
		"WARC-Record-ID: <urn:uuid:11111111-2222-3333-4444-555555555555>\r\n" +
		"Content-Type: application/http; msgtype=response\r\n"

	warcHeaders += "Content-Length: " + intToString(len(httpResponse)) + "\r\n" +
		"\r\n"

	// Trailing two CRLFs separate this record from the next.
	return []byte(warcHeaders + httpResponse + "\r\n\r\n")
}

// makeMultiRecordWARC builds a WARC with two HTTP response records and one
// metadata record. Used to test that IterRecords yields each one and that
// IterResponses filters out non-response records.
func makeMultiRecordWARC() []byte {
	page1Body := "<html><body>page1</body></html>"
	page2Body := "<html><body>page2</body></html>"
	metadataBody := "extracted: yes"

	build := func(warcType, uri string, body string) string {
		var content string
		if warcType == "response" {
			content = "HTTP/1.1 200 OK\r\n" +
				"Content-Type: text/html\r\n" +
				"Content-Length: " + intToString(len(body)) + "\r\n" +
				"\r\n" +
				body
		} else {
			content = body
		}
		return "WARC/1.0\r\n" +
			"WARC-Type: " + warcType + "\r\n" +
			"WARC-Target-URI: " + uri + "\r\n" +
			"WARC-Date: 2026-01-01T00:00:00Z\r\n" +
			"Content-Length: " + intToString(len(content)) + "\r\n" +
			"\r\n" +
			content + "\r\n\r\n"
	}

	return []byte(
		build("response", "https://example.com/page1", page1Body) +
			build("response", "https://example.com/page2", page2Body) +
			build("metadata", "https://example.com/page1", metadataBody),
	)
}

// intToString — tiny helper to avoid pulling in strconv at top of file.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestParseWARC_PlainBytes(t *testing.T) {
	parser, err := ParseWARC(makeWARC())
	if err != nil {
		t.Fatal(err)
	}
	if parser == nil {
		t.Fatal("ParseWARC returned nil parser")
	}
}

func TestParseWARC_GzipDecompresses(t *testing.T) {
	// Wrap our synthetic WARC in gzip — ParseWARC should auto-detect and decompress.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(makeWARC()); err != nil {
		t.Fatal(err)
	}
	gz.Close()

	parser, err := ParseWARC(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	pages, err := parser.GetPages()
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page after gzip decompression, got %d", len(pages))
	}
}

func TestWarcParser_IterRecordsYieldsResponse(t *testing.T) {
	parser, _ := ParseWARC(makeWARC())
	count := 0
	_, err := parser.IterRecords(func(r *WarcRecord) bool {
		count++
		if r.RecordType != "response" {
			t.Errorf("expected response, got %s", r.RecordType)
		}
		if r.URL != "https://example.com/page" {
			t.Errorf("URL: %s", r.URL)
		}
		if r.StatusCode != 200 {
			t.Errorf("StatusCode: %d", r.StatusCode)
		}
		if string(r.Content) != "<html></html>" {
			t.Errorf("Content: %q", string(r.Content))
		}
		if r.Headers["Content-Type"] != "text/html; charset=utf-8" {
			t.Errorf("HTTP Content-Type header: %q", r.Headers["Content-Type"])
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}
}

func TestWarcParser_GetPages(t *testing.T) {
	parser, _ := ParseWARC(makeMultiRecordWARC())
	pages, err := parser.GetPages()
	if err != nil {
		t.Fatal(err)
	}
	// Two response records, one metadata record → only 2 pages.
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].URL != "https://example.com/page1" {
		t.Errorf("page 0 URL: %s", pages[0].URL)
	}
	if pages[0].StatusCode != 200 {
		t.Errorf("page 0 status: %d", pages[0].StatusCode)
	}
	if !bytes.Contains(pages[0].Content, []byte("page1")) {
		t.Errorf("page 0 body: %q", string(pages[0].Content))
	}
	if pages[1].URL != "https://example.com/page2" {
		t.Errorf("page 1 URL: %s", pages[1].URL)
	}
}

func TestWarcParser_IterResponsesFiltersNonResponses(t *testing.T) {
	parser, _ := ParseWARC(makeMultiRecordWARC())
	responseCount := 0
	_, err := parser.IterResponses(func(r *WarcRecord) bool {
		responseCount++
		if r.RecordType != "response" {
			t.Errorf("got non-response: %s", r.RecordType)
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if responseCount != 2 {
		t.Errorf("expected 2 responses, got %d", responseCount)
	}
}

func TestWarcParser_EmptyInput(t *testing.T) {
	parser, _ := ParseWARC([]byte{})
	pages, err := parser.GetPages()
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages from empty WARC, got %d", len(pages))
	}
}

func TestExtractHTTPStatusCode(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"200 OK", "HTTP/1.1 200 OK\r\nfoo: bar\r\n\r\n", 200},
		{"404 Not Found", "HTTP/1.1 404 Not Found\r\n\r\n", 404},
		{"HTTP/1.0", "HTTP/1.0 301 Moved\r\n\r\n", 301},
		{"500 with body", "HTTP/1.1 500 Internal Server Error\r\nx: y\r\n\r\nbody", 500},
		{"no status line", "garbage\r\n", 0},
		{"empty", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractHTTPStatusCode([]byte(tc.body)); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseHTTPResponse_HeadersAndBody(t *testing.T) {
	content := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nX-Foo: bar\r\n\r\nhello world")
	headers, body := parseHTTPResponse(content)
	if headers["Content-Type"] != "text/plain" {
		t.Errorf("Content-Type: %q", headers["Content-Type"])
	}
	if headers["X-Foo"] != "bar" {
		t.Errorf("X-Foo: %q", headers["X-Foo"])
	}
	if string(body) != "hello world" {
		t.Errorf("body: %q", string(body))
	}
}
