// Scrape multiple URLs in a single request using the Batch Scraping API.
//
// ScrapeBatch accepts up to 100 ScrapeConfigs and streams each result
// through the returned channel as soon as it's ready. Results arrive
// OUT OF ORDER — use CorrelationID on every config to match each result
// back to its originating request.
package main

import (
	"fmt"
	"log"
	"os"

	scrapfly "github.com/scrapfly/go-scrapfly"
)

func main() {
	client, err := scrapfly.New(os.Getenv("SCRAPFLY_API_KEY"))
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Every config in a batch MUST carry a unique CorrelationID.
	configs := []*scrapfly.ScrapeConfig{
		{URL: "https://web-scraping.dev/product/1", CorrelationID: "product-1"},
		{URL: "https://web-scraping.dev/product/2", CorrelationID: "product-2"},
		{URL: "https://web-scraping.dev/product/3", CorrelationID: "product-3"},
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
			fmt.Printf("%s: proxified status=%d\n", r.CorrelationID, r.ProxifiedResponse.StatusCode)
			continue
		}
		fmt.Printf("%s: status=%d size=%d bytes\n",
			r.CorrelationID,
			r.Result.Result.StatusCode,
			len(r.Result.Result.Content),
		)
	}
}
