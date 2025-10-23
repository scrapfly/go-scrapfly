package main

import (
	"fmt"
	"log"

	"github.com/scrapfly/go-scrapfly"
)

func main() {
	key := "YOUR_SCRAPFLY_KEY"

	client, err := scrapfly.New(key)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Create a scrape configuration
	scrapeConfig := &scrapfly.ScrapeConfig{
		URL:       "https://web-scraping.dev/product/1",
		RenderJS:  true,
		Country:   "us",
		ASP:       true,
		ProxyPool: scrapfly.PublicResidentialPool,
	}

	// Perform the scrape
	apiResponse, err := client.Scrape(scrapeConfig)
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	// HTML content is in apiResponse.Result.Content
	//fmt.Println(apiResponse.Result.Content)

	// Use the built-in HTML parser (go-query)
	selector, err := apiResponse.Selector()
	if err != nil {
		log.Fatalf("failed to get selector: %v", err)
	}

	fmt.Println("Product Title:", selector.Find("h3").First().Text())
}
