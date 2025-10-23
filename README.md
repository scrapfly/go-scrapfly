# Scrapfly Go SDK

Go SDK for [Scrapfly.io](https://scrapfly.io/) web scraping API.

This SDK allows you to easily:
- Scrape the web without being blocked.
- Use headless browsers to access Javascript-powered page data.
- Take screenshots of websites.
- Extract structured data using AI.

For web scraping guides see [our blog](https://scrapfly.io/blog/) and [#scrapeguide](https://scrapfly.io/blog/tag/scrapeguide/) tag for how to scrape specific targets.

## Installation

```bash
go get github.com/scrapfly/go-scrapfly
```

## Quick Intro

1. Register a [Scrapfly account for free](https://scrapfly.io/register)
2. Get your API Key on [scrapfly.io/dashboard](https://scrapfly.io/dashboard)
3. Start scraping: 🚀

```go
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
		URL:        "https://web-scraping.dev/product/1",
		RenderJS:   true,
		Country:    "us",
		ASP:        true,
		ProxyPool:  scrapfly.PublicResidentialPool,
	}

	// Perform the scrape
	apiResponse, err := client.Scrape(scrapeConfig)
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	// HTML content is in apiResponse.Result.Content
	// fmt.Println(apiResponse.Result.Content)

	// Use the built-in HTML parser (go-query)
	selector, err := apiResponse.Selector()
	if err != nil {
		log.Fatalf("failed to get selector: %v", err)
	}
	
	fmt.Println("Product Title:", selector.Find("h3").First().Text())
}
```

## Debugging

To enable debug logs, you can set the logger level:

```go
scrapefly.Logger.SetLevel(scrapefly.LevelDebug)
```

Additionally, set `Debug: true` in `ScrapeConfig` to access debug information in the [Scrapfly web dashboard](https://scrapfly.io/dashboard):

```go
scrapeConfig := &scrapefly.ScrapeConfig{
    URL:   "https://web-scraping.dev/product/1",
    Debug: true, // Enable debug information for the web dashboard
}
```
