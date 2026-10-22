package scrapfly

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchWithRetryDoesNotRetryTimeout(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	client := &http.Client{Timeout: 50 * time.Millisecond}

	if _, err := fetchWithRetry(client, req, 3, time.Millisecond); err == nil {
		t.Fatal("expected the timeout error")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("a timed-out request was sent %d times, want 1", got)
	}
}

func TestFetchWithRetryRetriesServerError(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)

	if _, err := fetchWithRetry(server.Client(), req, 3, time.Millisecond); err == nil {
		t.Fatal("expected the server error")
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("a 502 was sent %d times, want 3", got)
	}
}

func TestDefaultTimeoutCoversScrapeReadTimeout(t *testing.T) {
	client, err := New("key")
	if err != nil {
		t.Fatal(err)
	}
	if client.HTTPClient().Timeout < 155*time.Second {
		t.Fatalf("default timeout %s is below the Scrape API's 155s read timeout", client.HTTPClient().Timeout)
	}
}
