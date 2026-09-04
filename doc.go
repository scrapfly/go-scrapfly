// Package scrapfly provides a Go SDK for the Scrapfly.io web scraping API.
//
// Scrapfly is a comprehensive web scraping API that handles proxies, browser rendering,
// anti-bot protection, and more. This SDK provides a simple and idiomatic Go interface
// to interact with the Scrapfly API for various web scraping tasks.
//
// # Features
//
//   - Web scraping with automatic proxy rotation
//   - JavaScript rendering with headless browsers
//   - Anti-bot protection bypass (Unblocker)
//   - Screenshot capture with multiple formats
//   - AI-powered structured data extraction
//   - Session management for persistent browsing
//   - Concurrent scraping with rate limiting
//   - Caching and webhook support
//
// # Installation
//
//	go get github.com/scrapfly/go-scrapfly
//
// # Quick Start
//
// Create a client and perform a simple scrape:
//
//	package main
//
//	import (
//	    "fmt"
//	    "log"
//	    "github.com/scrapfly/go-scrapfly"
//	)
//
//	func main() {
//	    // Create a new client
//	    client, err := scrapfly.New("YOUR_API_KEY")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Configure the scrape request
//	    config := &scrapfly.ScrapeConfig{
//	        URL:      "https://example.com",
//	        RenderJS: true,
//	        Country:  "us",
//	    }
//
//	    // Perform the scrape
//	    result, err := client.Scrape(config)
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Access the content
//	    fmt.Println(result.Result.Content)
//
//	    // Or use the built-in HTML parser
//	    doc, err := result.Selector()
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    title := doc.Find("title").First().Text()
//	    fmt.Println("Title:", title)
//	}
//
// # Advanced Features
//
// JavaScript Rendering:
//
//	config := &scrapfly.ScrapeConfig{
//	    URL:             "https://example.com",
//	    RenderJS:        true,
//	    WaitForSelector: ".content",
//	    RenderingWait:   2000,
//	    AutoScroll:      true,
//	}
//
// Unblocker (anti-bot bypass):
//
//	config := &scrapfly.ScrapeConfig{
//	    URL:       "https://example.com",
//	    Unblocker: scrapfly.BoolPtr(true),
//	}
//
// Unblocker is a *bool: nil leaves it unset, BoolPtr(true) enables the bypass
// and BoolPtr(false) explicitly disables it. ASP is the deprecated alias and
// keeps working — both fields are sent to the API as the same "asp" parameter,
// and ASP: true wins if both are set.
//
// Taking Screenshots:
//
//	config := &scrapfly.ScreenshotConfig{
//	    URL:        "https://example.com",
//	    Format:     scrapfly.FormatPNG,
//	    Capture:    "fullpage",
//	    Resolution: "1920x1080",
//	}
//	result, err := client.Screenshot(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	filePath, err := client.SaveScreenshot(result, "screenshot")
//
// Concurrent Scraping:
//
//	configs := []*scrapfly.ScrapeConfig{
//	    {URL: "https://example.com/page1"},
//	    {URL: "https://example.com/page2"},
//	    {URL: "https://example.com/page3"},
//	}
//	resultsChan := client.ConcurrentScrape(configs, 3)
//	for result := range resultsChan {
//	    if result.error != nil {
//	        log.Printf("Error: %v", result.error)
//	        continue
//	    }
//	    // Process result
//	}
//
// AI Data Extraction:
//
//	config := &scrapfly.ExtractionConfig{
//	    Body:             []byte("<html>...</html>"),
//	    ContentType:      "text/html",
//	    ExtractionPrompt: "Extract product name, price, and description",
//	}
//	result, err := client.Extract(config)
//
// # Error Handling
//
// The SDK uses sentinel errors that can be checked with errors.Is():
//
//	result, err := client.Scrape(config)
//	if err != nil {
//	    if errors.Is(err, scrapfly.ErrUpstreamClient) {
//	        // Target website returned 4xx error
//	    } else if errors.Is(err, scrapfly.ErrProxyFailed) {
//	        // Proxy connection failed
//	    } else if errors.Is(err, scrapfly.ErrUnblockerBypassFailed) {
//	        // The unblocker could not get through the target's protection.
//	        // Same value as the deprecated ErrASPBypassFailed.
//	    } else if apiErr, ok := err.(*scrapfly.APIError); ok {
//	        // Get detailed API error information
//	        fmt.Printf("Status: %d, Message: %s\n", apiErr.HTTPStatusCode, apiErr.Message)
//	    }
//	}
//
// # Debugging
//
// Enable debug logging to see detailed request information:
//
//	scrapfly.DefaultLogger.SetLevel(scrapfly.LevelDebug)
//
// Enable debug mode in the API to access debug information in the dashboard:
//
//	config := &scrapfly.ScrapeConfig{
//	    URL:   "https://example.com",
//	    Debug: true,
//	}
//
// # Documentation
//
// For more information, visit:
//   - API Documentation: https://scrapfly.io/docs
//   - Web Scraping Guides: https://scrapfly.io/blog/tag/scrapeguide/
//   - Dashboard: https://scrapfly.io/dashboard
package scrapfly
