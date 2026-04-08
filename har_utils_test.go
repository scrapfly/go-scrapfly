package scrapfly

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// makeStandardHAR builds a small HAR file in the standard format:
// `{"log": {"version": "1.2", "creator": {...}, "entries": [...]}}`
func makeStandardHAR() []byte {
	return []byte(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "test", "version": "1.0"},
			"pages": [{"id": "page_1", "title": "Home"}],
			"entries": [
				{
					"startedDateTime": "2026-01-01T00:00:00Z",
					"time": 123.4,
					"timings": {"dns": 5.0, "wait": 50.0, "receive": 68.4},
					"request": {
						"method": "GET",
						"url": "https://example.com/page1",
						"headers": [
							{"name": "Accept", "value": "text/html"},
							{"name": "User-Agent", "value": "test/1.0"}
						]
					},
					"response": {
						"status": 200,
						"statusText": "OK",
						"headers": [
							{"name": "Content-Type", "value": "text/html; charset=utf-8"},
							{"name": "X-Foo", "value": "bar"}
						],
						"content": {
							"size": 13,
							"mimeType": "text/html",
							"text": "<html></html>"
						}
					}
				},
				{
					"startedDateTime": "2026-01-01T00:00:01Z",
					"time": 50,
					"request": {
						"method": "POST",
						"url": "https://example.com/api",
						"headers": []
					},
					"response": {
						"status": 404,
						"statusText": "Not Found",
						"headers": [],
						"content": {
							"size": 9,
							"mimeType": "application/json",
							"text": "not found"
						}
					}
				}
			]
		}
	}`)
}

// makeStreamingHAR builds the Scrapfly streaming HAR variant:
// a leading `log` envelope (with empty entries) followed by individual
// entry objects, all concatenated.
func makeStreamingHAR() []byte {
	return []byte(`{
		"log": {
			"version": "1.2",
			"creator": {"name": "scrapfly-streaming", "version": "1.0"},
			"entries": []
		}
	}
	{
		"startedDateTime": "2026-01-01T00:00:00Z",
		"time": 100,
		"request": {"method": "GET", "url": "https://example.com/a", "headers": []},
		"response": {
			"status": 200,
			"statusText": "OK",
			"headers": [],
			"content": {"size": 1, "mimeType": "text/plain", "text": "a"}
		}
	}
	{
		"startedDateTime": "2026-01-01T00:00:01Z",
		"time": 200,
		"request": {"method": "GET", "url": "https://example.com/b", "headers": []},
		"response": {
			"status": 200,
			"statusText": "OK",
			"headers": [],
			"content": {"size": 1, "mimeType": "text/plain", "text": "b"}
		}
	}`)
}

func TestParseHAR_Standard(t *testing.T) {
	archive, err := ParseHAR(makeStandardHAR())
	if err != nil {
		t.Fatal(err)
	}
	if archive.Version() != "1.2" {
		t.Errorf("version: %s", archive.Version())
	}
	if archive.Len() != 2 {
		t.Errorf("expected 2 entries, got %d", archive.Len())
	}
	creator := archive.Creator()
	if creator["name"] != "test" {
		t.Errorf("creator.name: %v", creator["name"])
	}
	pages := archive.Pages()
	if len(pages) != 1 {
		t.Errorf("expected 1 page, got %d", len(pages))
	}
}

func TestParseHAR_Streaming(t *testing.T) {
	archive, err := ParseHAR(makeStreamingHAR())
	if err != nil {
		t.Fatal(err)
	}
	if archive.Len() != 2 {
		t.Errorf("expected 2 entries, got %d", archive.Len())
	}
	if archive.Creator()["name"] != "scrapfly-streaming" {
		t.Errorf("creator: %v", archive.Creator())
	}
	urls := archive.URLs()
	if len(urls) != 2 || urls[0] != "https://example.com/a" {
		t.Errorf("urls: %v", urls)
	}
}

func TestParseHAR_GzipDecompresses(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(makeStandardHAR()); err != nil {
		t.Fatal(err)
	}
	gz.Close()

	archive, err := ParseHAR(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if archive.Len() != 2 {
		t.Errorf("expected 2 entries from gzipped HAR, got %d", archive.Len())
	}
}

func TestHarEntry_Accessors(t *testing.T) {
	archive, _ := ParseHAR(makeStandardHAR())
	entries := archive.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	first := entries[0]
	if first.URL() != "https://example.com/page1" {
		t.Errorf("URL: %s", first.URL())
	}
	if first.Method() != "GET" {
		t.Errorf("Method: %s", first.Method())
	}
	if first.StatusCode() != 200 {
		t.Errorf("StatusCode: %d", first.StatusCode())
	}
	if first.StatusText() != "OK" {
		t.Errorf("StatusText: %s", first.StatusText())
	}
	if first.ContentType() != "text/html" {
		t.Errorf("ContentType: %s", first.ContentType())
	}
	if string(first.Content()) != "<html></html>" {
		t.Errorf("Content: %q", string(first.Content()))
	}
	if first.ContentSize() != 13 {
		t.Errorf("ContentSize: %d", first.ContentSize())
	}
	if first.Time() != 123.4 {
		t.Errorf("Time: %f", first.Time())
	}
	if first.StartedDateTime() != "2026-01-01T00:00:00Z" {
		t.Errorf("StartedDateTime: %s", first.StartedDateTime())
	}

	reqHeaders := first.RequestHeaders()
	if reqHeaders["Accept"] != "text/html" {
		t.Errorf("RequestHeaders[Accept]: %s", reqHeaders["Accept"])
	}
	if reqHeaders["User-Agent"] != "test/1.0" {
		t.Errorf("RequestHeaders[User-Agent]: %s", reqHeaders["User-Agent"])
	}

	respHeaders := first.ResponseHeaders()
	if respHeaders["Content-Type"] != "text/html; charset=utf-8" {
		t.Errorf("ResponseHeaders[Content-Type]: %s", respHeaders["Content-Type"])
	}

	timings := first.Timings()
	if timings["dns"] != 5.0 {
		t.Errorf("timings.dns: %f", timings["dns"])
	}
}

func TestHarEntry_Base64Content(t *testing.T) {
	body := `{
		"log": {
			"version": "1.2",
			"creator": {"name": "test", "version": "1.0"},
			"entries": [{
				"startedDateTime": "2026-01-01T00:00:00Z",
				"time": 1,
				"request": {"method": "GET", "url": "https://example.com/img", "headers": []},
				"response": {
					"status": 200, "statusText": "OK", "headers": [],
					"content": {"size": 4, "mimeType": "image/png", "encoding": "base64", "text": "SGVsbG8="}
				}
			}]
		}
	}`
	archive, err := ParseHAR([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if archive.Len() != 1 {
		t.Fatal("expected 1 entry")
	}
	got := archive.Entries()[0].Content()
	if string(got) != "Hello" {
		t.Errorf("expected base64-decoded 'Hello', got %q", string(got))
	}
}

func TestHarArchive_FindByURL(t *testing.T) {
	archive, _ := ParseHAR(makeStandardHAR())
	entry := archive.FindByURL("https://example.com/api")
	if entry == nil {
		t.Fatal("expected to find entry by URL")
	}
	if entry.Method() != "POST" {
		t.Errorf("Method: %s", entry.Method())
	}
	missing := archive.FindByURL("https://example.com/nope")
	if missing != nil {
		t.Error("expected nil for missing URL")
	}
}

func TestHarArchive_FilterByStatus(t *testing.T) {
	archive, _ := ParseHAR(makeStandardHAR())
	notFound := archive.FilterByStatus(404)
	if len(notFound) != 1 {
		t.Errorf("expected 1 404 entry, got %d", len(notFound))
	}
	if notFound[0].URL() != "https://example.com/api" {
		t.Errorf("404 URL: %s", notFound[0].URL())
	}
}

func TestHarArchive_FilterByContentType(t *testing.T) {
	archive, _ := ParseHAR(makeStandardHAR())
	html := archive.FilterByContentType("text/html")
	if len(html) != 1 {
		t.Errorf("expected 1 text/html entry, got %d", len(html))
	}
	json := archive.FilterByContentType("application/json")
	if len(json) != 1 {
		t.Errorf("expected 1 application/json entry, got %d", len(json))
	}
}

func TestHarArchive_IterEntries(t *testing.T) {
	archive, _ := ParseHAR(makeStandardHAR())
	count := 0
	archive.IterEntries(func(*HarEntry) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("expected 2 iterations, got %d", count)
	}
}

func TestHarArchive_IterEntriesEarlyStop(t *testing.T) {
	archive, _ := ParseHAR(makeStandardHAR())
	count := 0
	archive.IterEntries(func(*HarEntry) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Errorf("expected 1 iteration with early stop, got %d", count)
	}
}

func TestParseHAR_RejectsNonHAR(t *testing.T) {
	_, err := ParseHAR([]byte(`{"foo": "bar"}`))
	if err == nil {
		t.Fatal("expected error for missing 'log' field")
	}
}
