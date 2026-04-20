// Use msgpack wire encoding for per-part bodies in a batch.
//
// Msgpack produces slightly smaller payloads than JSON and decodes
// faster. Pass BatchOptions{Format: BatchFormatMsgpack} via
// ScrapeBatchWithOptions to opt in — the SDK handles decoding
// transparently.
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

	configs := []*scrapfly.ScrapeConfig{
		{URL: "https://web-scraping.dev/product/1", CorrelationID: "product-1"},
		{URL: "https://web-scraping.dev/product/2", CorrelationID: "product-2"},
	}

	results, err := client.ScrapeBatchWithOptions(configs, scrapfly.BatchOptions{
		Format: scrapfly.BatchFormatMsgpack,
	})
	if err != nil {
		log.Fatalf("ScrapeBatchWithOptions: %v", err)
	}

	for r := range results {
		if r.Err != nil {
			fmt.Printf("%s: error %v\n", r.CorrelationID, r.Err)
			continue
		}
		fmt.Printf("%s: status=%d\n", r.CorrelationID, r.Result.Result.StatusCode)
	}
}
