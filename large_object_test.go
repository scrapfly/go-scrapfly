package scrapfly

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// largeObjectAPI answers /scrape with a large object envelope and serves the object itself,
// gzipped as the API compresses a JSON or msgpack answer when the request accepts it.
func largeObjectAPI(t *testing.T, format, contentType string, body []byte, status int) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scrape" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"config":  map[string]any{"url": "https://web-scraping.dev/product/1"},
				"context": map[string]any{},
				"result": map[string]any{
					"status": "DONE", "success": true, "status_code": 200,
					"format": format, "content": server.URL + "/scrape/large_object/01",
				},
			})
			return
		}
		if r.URL.Query().Get("key") != "scp-test-key" {
			t.Errorf("large object fetched without the API key")
		}
		w.Header().Set("Content-Type", contentType)
		if strings.HasPrefix(contentType, "application/json") && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(status)
			gz := gzip.NewWriter(w)
			_, _ = gz.Write(body)
			_ = gz.Close()
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestLargeObjectIsReturnedAsServed(t *testing.T) {
	cases := []struct {
		name, format, contentType string
		body                      []byte
		wantFormat                string
	}{
		{"json clob", "clob", "application/json; charset=utf-8", []byte(`{"products": [{"id": 1}]}`), "text"},
		{"html clob", "clob", "text/html; charset=utf-8", []byte("<h1>large</h1>"), "text"},
		{"json blob", "blob", "application/json", []byte(`{"products": [{"id": 1}]}`), "binary"},
		{"pdf blob", "blob", "application/pdf", []byte("%PDF-\x00\xff\x8b"), "binary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := largeObjectAPI(t, tc.format, tc.contentType, tc.body, http.StatusOK)
			client, err := NewWithHost("scp-test-key", server.URL, true)
			if err != nil {
				t.Fatal(err)
			}

			result, err := client.Scrape(&ScrapeConfig{URL: "https://web-scraping.dev/product/1"})
			if err != nil {
				t.Fatalf("scrape: %v", err)
			}
			if result.Result.Content != string(tc.body) {
				t.Fatalf("content = %q, want %q", result.Result.Content, tc.body)
			}
			if result.Result.Format != tc.wantFormat {
				t.Fatalf("format = %q, want %q", result.Result.Format, tc.wantFormat)
			}
		})
	}
}

func TestLargeObjectErrorStatusIsAnError(t *testing.T) {
	server := largeObjectAPI(t, "clob", "application/json", []byte(`{"reason":"Not Found","message":"Large Object 01 not found","error_id":"01","http_code":404}`), http.StatusNotFound)
	client, err := NewWithHost("scp-test-key", server.URL, true)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Scrape(&ScrapeConfig{URL: "https://web-scraping.dev/product/1"})
	if err == nil || !strings.Contains(err.Error(), "Large Object 01 not found") {
		t.Fatalf("err = %v, want the large object's 404", err)
	}
	if strings.Contains(err.Error(), "scp-test-key") {
		t.Fatalf("error prints the API key: %v", err)
	}
}
