package scrapfly

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const scenarioTarget = "https://web-scraping.dev/products"

func assertScenarioRenderingError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrScrapeConfig) || !strings.Contains(err.Error(), "js_scenario requires render_js=true") {
		t.Fatalf("expected scenario rendering error, got %v", err)
	}
}

func TestScrapeScenarioRequiresRendering(t *testing.T) {
	requests := make(chan url.Values, 8)
	client := newTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		requests <- req.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{"success":true,"status":"DONE","status_code":200}}`)
	})

	config := &ScrapeConfig{URL: scenarioTarget, CorrelationID: "scenario-fixture", JSScenario: []map[string]any{{"wait": 50}}}
	_, err := client.Scrape(config)
	assertScenarioRenderingError(t, err)
	proxified := *config
	_, err = client.ScrapeProxified(&proxified)
	assertScenarioRenderingError(t, err)
	_, err = client.ScrapeBatchWithOptions([]*ScrapeConfig{config}, BatchOptions{})
	assertScenarioRenderingError(t, err)
	if len(requests) != 0 {
		t.Fatalf("invalid scenarios sent %d requests", len(requests))
	}

	config.RenderJS = true
	if _, err := client.Scrape(config); err != nil || len(requests) != 1 {
		t.Fatalf("rendered scenario: error=%v requests=%d", err, len(requests))
	}
	query := <-requests
	wantScenario := base64.RawURLEncoding.EncodeToString([]byte(`[{"wait":50}]`))
	if query.Get("render_js") != "true" || query.Get("js_scenario") != wantScenario {
		t.Fatalf("render_js=%q js_scenario=%q", query.Get("render_js"), query.Get("js_scenario"))
	}

	for _, scenario := range [][]map[string]any{nil, {}} {
		config := &ScrapeConfig{URL: scenarioTarget, JSScenario: scenario}
		if _, err := client.Scrape(config); err != nil || len(requests) != 1 {
			t.Fatalf("no scenario: error=%v requests=%d", err, len(requests))
		}
		if query := <-requests; query.Has("js_scenario") || query.Has("render_js") {
			t.Fatalf("no scenario sent unexpected rendering parameters: %v", query)
		}
	}
}

func TestConcurrentScrapeValidatesScenariosBeforeAccountLookup(t *testing.T) {
	requests := make(chan string, 4)
	client := newTestClient(t, func(w http.ResponseWriter, req *http.Request) {
		requests <- req.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/account":
			_, _ = io.WriteString(w, `{"subscription":{"usage":{"scrape":{"concurrent_limit":1}}}}`)
		case "/scrape":
			_, _ = io.WriteString(w, `{"result":{"success":true,"status":"DONE","status_code":200}}`)
		default:
			http.NotFound(w, req)
		}
	})
	invalid := &ScrapeConfig{URL: scenarioTarget, JSScenario: []map[string]any{{"wait": 50}}}
	failed := 0
	for result := range client.ConcurrentScrape([]*ScrapeConfig{invalid}, 0) {
		assertScenarioRenderingError(t, result.Error)
		failed++
	}
	if failed != 1 || len(requests) != 0 {
		t.Fatalf("invalid-only batch: errors=%d requests=%d", failed, len(requests))
	}

	successes := 0
	for result := range client.ConcurrentScrape([]*ScrapeConfig{invalid, {URL: scenarioTarget}}, 0) {
		if result.Error != nil {
			assertScenarioRenderingError(t, result.Error)
			failed++
		} else if result.Result != nil {
			successes++
		}
	}
	if failed != 2 || successes != 1 || len(requests) != 2 {
		t.Fatalf("mixed batch: errors=%d successes=%d requests=%d", failed-1, successes, len(requests))
	}
	if first, second := <-requests, <-requests; first != "/account" || second != "/scrape" {
		t.Fatalf("mixed batch sent %s and %s", first, second)
	}
}
