# Scrapfly Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/scrapfly/go-scrapfly.svg)](https://pkg.go.dev/github.com/scrapfly/go-scrapfly)

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
		Unblocker:  scrapfly.BoolPtr(true),
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

## Unblocker

`Unblocker` turns on Scrapfly's anti-bot bypass. It is a `*bool`, so use
`scrapfly.BoolPtr(true)` to enable it and `scrapfly.BoolPtr(false)` to
explicitly disable it; leaving it `nil` means "unset".

```go
scrapeConfig := &scrapfly.ScrapeConfig{
	URL:       "https://web-scraping.dev/product/1",
	Unblocker: scrapfly.BoolPtr(true),
}

crawlerConfig := &scrapfly.CrawlerConfig{
	URL:       "https://web-scraping.dev/",
	Unblocker: scrapfly.BoolPtr(true),
}
```

`ASP` is the deprecated alias of `Unblocker` and keeps working — existing code
needs no change. Only the name changed: both fields are sent to the API as the
same `asp` parameter. When both are set, `ASP: true` wins, since a plain `bool`
cannot tell "unset" from "explicitly false".

That same limitation makes one row diverge from the other Scrapfly SDKs. Since
`ASP: false` is indistinguishable from an unset `ASP`, the pair
`ASP: false, Unblocker: scrapfly.BoolPtr(true)` turns the feature **on** here,
where the Python, TypeScript and Rust SDKs let the explicit `false` win.
Retyping `ASP` to `*bool` would fix it but would stop every existing caller from
compiling, so it stays a `bool`. To turn the feature off, leave `Unblocker` nil
or pass `scrapfly.BoolPtr(false)` — do not write that contradictory pair.

The two names are also two **independent fields**, where the other SDKs expose
one storage slot under two names. Writing one never updates the other, so:

```go
cfg.ASP = true
cfg.Unblocker = scrapfly.BoolPtr(false) // does NOT turn it off — ASP still wins
cfg.Unblocker                            // nil after cfg.ASP = true
```

Last write does not win, and reading one name back after writing the other gives
the zero value. To turn the feature off, clear `ASP`. To read what the request
will actually carry, call `UnblockerEnabled()`:

```go
cfg := &scrapfly.ScrapeConfig{URL: "https://web-scraping.dev/", Unblocker: scrapfly.BoolPtr(true)}
cfg.UnblockerEnabled() // true — the value that goes out as `asp`
```

The matching error sentinel is `scrapfly.ErrUnblockerBypassFailed`, the same
value as the deprecated `scrapfly.ErrASPBypassFailed`, so `errors.Is` matches
either name:

```go
if errors.Is(err, scrapfly.ErrUnblockerBypassFailed) {
	// the unblocker could not get through the target's protection
}
```

## Full Documentation
* Please refer to the [Scrapfly API documentation](https://scrapfly.io/docs) for full documentation and examples.