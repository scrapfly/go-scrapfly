package scrapfly

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

const promptSearchEnvelope = `{"completeness":"exact","crawls_requested":2,"crawls_searched":1,"crawls_pruned_exact":0,` +
	`"skipped":[{"crawler_uuid":"0198bbbb","reason":"search_not_enabled"}]}`

func TestClient_CrawlsPromptAnswer_SendsStreamFalseOnce(t *testing.T) {
	var got map[string]interface{}
	var calls int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/crawl/prompt" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("path %s, Accept %s", r.URL.Path, r.Header.Get("Accept"))
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prompt":"What does it sell?","answer":"Chocolate boxes.",` +
			`"sources":[{"id":1,"crawler_uuid":"0198aaaa","url":"https://web-scraping.dev/product/1","title":"Box","score":0.9}],` +
			`"sources_used":[1],"sources_dropped":0,"truncated":false,"search":` + promptSearchEnvelope + `,"api_credit":5}`))
	})

	answer, err := client.CrawlsPromptAnswer([]string{"0198aaaa", "0198bbbb"}, "What does it sell?",
		&CrawlPromptOptions{Search: &CrawlSearchOptions{Limit: 5}, Model: "gemini-3.5-flash-lite"})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]interface{}{
		"prompt":     "What does it sell?",
		"crawl_ids":  []interface{}{"0198aaaa", "0198bbbb"},
		"generation": map[string]interface{}{"stream": false, "model": "gemini-3.5-flash-lite"},
		"search":     map[string]interface{}{"limit": float64(5)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("body %v, want %v", got, want)
	}
	if calls != 1 {
		t.Errorf("sent %d times, want once: the answer is billed", calls)
	}
	if answer.Answer != "Chocolate boxes." || answer.Sources[0].URL != "https://web-scraping.dev/product/1" {
		t.Errorf("answer %+v", answer)
	}
	if answer.Search.Completeness != "exact" || answer.Search.Skipped[0].Reason != "search_not_enabled" {
		t.Errorf("search %+v", answer.Search)
	}
	if answer.APICredit == nil || *answer.APICredit != 5 {
		t.Errorf("api_credit %v", answer.APICredit)
	}
}

func TestClient_CrawlsExtract_SendsSchemaAndDecodesData(t *testing.T) {
	var got map[string]interface{}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"products": map[string]interface{}{"type": "array"}},
		"required":   []interface{}{"products"},
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crawl/extract" {
			t.Errorf("path %s", r.URL.Path)
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prompt":"Every product with its price","data":{"products":[{"name":"Box of Chocolate Candy","price":9.99}]},` +
			`"sources":[],"sources_used":[],"sources_dropped":0,"truncated":false,"search":` + promptSearchEnvelope + `}`))
	})

	res, err := client.CrawlsExtract([]string{"0198aaaa"}, "Every product with its price", schema,
		&CrawlExtractOptions{Search: &CrawlSearchOptions{Filters: map[string]interface{}{"url_prefix": "https://web-scraping.dev/product/"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got["schema"], schema) || got["prompt"] != "Every product with its price" {
		t.Errorf("body %v", got)
	}
	if got["search"].(map[string]interface{})["filters"] == nil {
		t.Errorf("retrieval filters dropped: %v", got)
	}

	var out struct {
		Products []struct {
			Name  string  `json:"name"`
			Price float64 `json:"price"`
		} `json:"products"`
	}
	if err := res.Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Products) != 1 || out.Products[0].Price != 9.99 {
		t.Errorf("data %+v", out)
	}
}

func TestClient_CrawlsExtract_ErrorEnvelopeWraps(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"ERR::CRAWLER::CONFIG_ERROR","message":"A non-empty schema must be given"}`))
	})
	_, err := client.CrawlsExtract([]string{"0198aaaa"}, "products", map[string]interface{}{"type": "object"}, nil)
	if !errors.Is(err, ErrCrawlerFailed) || !strings.Contains(err.Error(), "CONFIG_ERROR") {
		t.Fatalf("expected a wrapped ERR::CRAWLER::CONFIG_ERROR, got %v", err)
	}
}

func TestClient_CrawlsExtract_EngineFailureKeepsItsMessage(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"the model returned no text","code":"ERR::CRAWLER::PROMPT_GENERATION_FAILED"}`))
	})
	_, err := client.CrawlsExtract([]string{"0198aaaa"}, "products", map[string]interface{}{"type": "object"}, nil)
	if !errors.Is(err, ErrCrawlerFailed) {
		t.Fatalf("expected ErrCrawlerFailed, got %v", err)
	}
	for _, part := range []string{"the model returned no text", "ERR::CRAWLER::PROMPT_GENERATION_FAILED", "status: 500"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error %q lacks %q", err, part)
		}
	}
}

func TestClient_CrawlsExtractAndPromptAnswer_RejectBadInputBeforeRequest(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should be issued")
	})
	schema := map[string]interface{}{"type": "object"}

	for name, err := range map[string]error{
		"extract empty crawl_ids": func() error { _, e := client.CrawlsExtract(nil, "p", schema, nil); return e }(),
		"extract empty prompt":    func() error { _, e := client.CrawlsExtract([]string{"a"}, "", schema, nil); return e }(),
		"extract empty schema":    func() error { _, e := client.CrawlsExtract([]string{"a"}, "p", nil, nil); return e }(),
		"extract bad mode": func() error {
			_, e := client.CrawlsExtract([]string{"a"}, "p", schema, &CrawlExtractOptions{Search: &CrawlSearchOptions{Mode: "semantic"}})
			return e
		}(),
		"extract cursor": func() error {
			_, e := client.CrawlsExtract([]string{"a"}, "p", schema, &CrawlExtractOptions{Search: &CrawlSearchOptions{Cursor: "next"}})
			return e
		}(),
		"answer cursor": func() error {
			_, e := client.CrawlsPromptAnswer([]string{"a"}, "p", &CrawlPromptOptions{Search: &CrawlSearchOptions{Cursor: "next"}})
			return e
		}(),
		"stream cursor": client.CrawlsPrompt([]string{"a"}, "p", &CrawlPromptOptions{Search: &CrawlSearchOptions{Cursor: "next"}},
			func(CrawlerPromptEvent) error { return nil }),
		"answer empty crawl_ids": func() error { _, e := client.CrawlsPromptAnswer(nil, "p", nil); return e }(),
		"answer empty prompt":    func() error { _, e := client.CrawlsPromptAnswer([]string{"a"}, "", nil); return e }(),
		"answer empty uuid":      func() error { _, e := client.CrawlPromptAnswer("", "p", nil); return e }(),
		"extract empty uuid":     func() error { _, e := client.CrawlExtract("", "p", schema, nil); return e }(),
	} {
		if !errors.Is(err, ErrCrawlerConfig) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCrawl_ExtractAndPromptAnswer_SendTheCrawlAlone(t *testing.T) {
	var sent []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(bodyBytes, &body)
		sent = append(sent, r.URL.Path+" "+strings.Join(toStrings(body["crawl_ids"]), ","))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prompt":"q","answer":"a","data":{},"sources":[],"sources_used":[],"sources_dropped":0,"truncated":false,"search":` + promptSearchEnvelope + `}`))
	})
	crawl := &Crawl{client: client, uuid: "0198aaaa"}

	if _, err := crawl.PromptAnswer("q", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := crawl.Extract("q", map[string]interface{}{"type": "object"}, nil); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/crawl/prompt 0198aaaa", "/crawl/extract 0198aaaa"}; !reflect.DeepEqual(sent, want) {
		t.Errorf("sent %v, want %v", sent, want)
	}
}

func toStrings(v interface{}) []string {
	var out []string
	for _, item := range v.([]interface{}) {
		out = append(out, item.(string))
	}
	return out
}
