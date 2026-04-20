// Mix proxified and JSON-envelope scrapes in a single batch.
//
// A config with ProxifiedResponse=true returns the raw upstream response
// (HTML, JSON, binary, etc.) instead of Scrapfly's JSON envelope. In a
// batch, proxified parts surface as an *http.Response on
// BatchResult.ProxifiedResponse; normal parts surface as *ScrapeResult
// on BatchResult.Result.
package main

import (
	"fmt"
	"io"
	"log"
	"os"

	scrapfly "github.com/scrapfly/go-scrapfly"
)

func main() {
	client, err := scrapfly.New(os.Getenv("SCRAPFLY_API_KEY"))
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	configs := []*scrapfly.ScrapeConfig{
		// Proxified: raw upstream HTML + upstream headers + X-Scrapfly-* metadata.
		{
			URL:               "https://web-scraping.dev/product/1",
			CorrelationID:     "html",
			ProxifiedResponse: true,
		},
		// Normal: Scrapfly JSON envelope with config, context, result.
		{URL: "https://web-scraping.dev/api/products", CorrelationID: "api"},
	}

	results, err := client.ScrapeBatch(configs)
	if err != nil {
		log.Fatalf("ScrapeBatch: %v", err)
	}

	for r := range results {
		if r.Err != nil {
			fmt.Printf("%s: error %v\n", r.CorrelationID, r.Err)
			continue
		}
		if r.ProxifiedResponse != nil {
			body, _ := io.ReadAll(r.ProxifiedResponse.Body)
			_ = r.ProxifiedResponse.Body.Close()
			fmt.Printf("%s: proxified status=%d content-type=%s body=%d bytes\n",
				r.CorrelationID,
				r.ProxifiedResponse.StatusCode,
				r.ProxifiedResponse.Header.Get("Content-Type"),
				len(body),
			)
			continue
		}
		fmt.Printf("%s: scrape status=%d\n", r.CorrelationID, r.Result.Result.StatusCode)
	}
}
