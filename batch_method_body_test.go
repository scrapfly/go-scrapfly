package scrapfly

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestScrapeBatch_SendsEachConfigsMethodAndBody(t *testing.T) {
	var sent struct {
		Configs []map[string]string `json:"configs"`
	}

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &sent); err != nil {
			t.Errorf("batch body: %v", err)
		}

		w.Header().Set("Content-Type", "multipart/mixed; boundary=b0und")
		for _, id := range []string{"post", "get"} {
			part := fmt.Sprintf(`{"config":{"url":"https://web-scraping.dev/"},"context":{},"result":{"status":"DONE","success":true,"status_code":200,"content":"%s","format":"text"}}`, id)
			fmt.Fprintf(w, "--b0und\r\nContent-Type: application/json\r\nX-Scrapfly-Correlation-Id: %s\r\nContent-Length: %d\r\n\r\n%s\r\n", id, len(part), part)
		}
		fmt.Fprint(w, "--b0und--\r\n")
	})

	results, err := client.ScrapeBatch([]*ScrapeConfig{
		{URL: "https://web-scraping.dev/api", Method: HttpMethodPost, Body: `{"q":1}`, Headers: map[string]string{"content-type": "application/json"}, CorrelationID: "post"},
		{URL: "https://web-scraping.dev/page", CorrelationID: "get"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range results {
	}

	if len(sent.Configs) != 2 {
		t.Fatalf("configs sent: %d", len(sent.Configs))
	}
	if sent.Configs[0]["method"] != "POST" || sent.Configs[0]["body"] != `{"q":1}` {
		t.Errorf("the POST config went out as method=%q body=%q", sent.Configs[0]["method"], sent.Configs[0]["body"])
	}
	if sent.Configs[1]["method"] != "GET" {
		t.Errorf("the GET config went out as method=%q", sent.Configs[1]["method"])
	}
	if _, ok := sent.Configs[1]["body"]; ok {
		t.Errorf("the GET config carries a body")
	}
}
