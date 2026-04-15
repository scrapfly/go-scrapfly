package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/scrapfly/go-scrapfly"
	js_scenario "github.com/scrapfly/go-scrapfly/scenario"
)

// getAccount demonstrates fetching account information
func getAccount(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	account, err := client.Account()
	if err != nil {
		log.Fatalf("failed to get account: %v", err)
	}

	fmt.Println("Account:")
	accountJSON, _ := json.MarshalIndent(account, "", "  ")
	fmt.Println(string(accountJSON))
}

// basicGet demonstrates basic scraping with cache and ASP
func basicGet(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL: "https://httpbin.dev/html",
		// Anti Scraping Protection bypass - enable this when scraping protected targets
		ASP: true,
		// server side cache - great for repeated requests
		Cache:    true,
		CacheTTL: 3600, // in seconds
		// CacheClear: true,  // you can always clear the cache explicitly!
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	// the scrape_result.Result contains all result details
	fmt.Println("web log url:") // you can check web UI for request details:
	fmt.Println(scrapeResult.Result.LogURL)

	fmt.Println("\npage content:")
	fmt.Println(scrapeResult.Result.Content)

	fmt.Println("\nresponse headers:")
	headersJSON, _ := json.MarshalIndent(scrapeResult.Result.ResponseHeaders, "", "  ")
	fmt.Println(string(headersJSON))

	fmt.Println("\nresponse cookies:")
	cookiesJSON, _ := json.MarshalIndent(scrapeResult.Result.Cookies, "", "  ")
	fmt.Println(string(cookiesJSON))
}

// jsRender demonstrates JavaScript rendering with scenarios
func jsRender(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Build a JavaScript scenario using the scenario builder
	scenario, err := js_scenario.New().
		Click("#load-more-reviews").
		Wait(2000).
		Build()
	if err != nil {
		log.Fatalf("failed to build scenario: %v", err)
	}

	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL: "https://web-scraping.dev/product/1",
		// enable browsers:
		RenderJS: true,
		// this enables more options
		// you can wait for some element to appear:
		WaitForSelector: ".review",
		// you can wait explicitly for N seconds
		RenderingWait: 3000, // 3 seconds
		// you can control the browser through scenarios:
		// https://scrapfly.io/docs/scrape-api/javascript-scenario
		JSScenario: scenario,
		// or even run any custom JS code!
		JS: `return document.querySelector(".review").innerText`,
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	// the scrape_result.Result contains all result details:
	fmt.Println("web log url:") // you can check web UI for request details:
	fmt.Println(scrapeResult.Result.LogURL)

	fmt.Println("\npage content (first 1000 chars):")
	content := scrapeResult.Result.Content
	if len(content) > 1000 {
		content = content[:1000] + "..."
	}
	fmt.Println(content)

	fmt.Println("\nbrowser data capture:")
	browserDataJSON, _ := json.MarshalIndent(scrapeResult.Result.BrowserData, "", "  ")
	fmt.Println(string(browserDataJSON))
}

// scrapeExtraction demonstrates scraping with inline extraction using LLM prompts
func scrapeExtraction(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL: "https://web-scraping.dev/product/1",
		// enable browsers:
		RenderJS: true,
		// use LLM prompt for auto parsing
		ExtractionPrompt: "Extract the product specification in json format",
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	// access the extraction result
	fmt.Println("extraction result:")
	extractedDataJSON, _ := json.MarshalIndent(scrapeResult.Result.ExtractedData, "", "  ")
	fmt.Println(string(extractedDataJSON))
}

// extractionLLM demonstrates using the extraction API with LLM prompts
func extractionLLM(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// First, get HTML either from Web Scraping API or your own storage
	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL: "https://web-scraping.dev/product/1",
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}
	html := scrapeResult.Result.Content

	// LLM Parsing - extract content using LLM queries
	llmResult, err := client.Extract(&scrapfly.ExtractionConfig{
		// identify content type like text/html or text/markdown etc.
		ContentType: "text/html",
		Body:        []byte(html),
		// use any prompt
		ExtractionPrompt: "get product price only",
	})
	if err != nil {
		log.Fatalf("extraction failed: %v", err)
	}

	fmt.Println("llm extraction:")
	llmResultJSON, _ := json.MarshalIndent(llmResult, "", "  ")
	fmt.Println(string(llmResultJSON))

	// You can also request LLM to output specific formats like JSON or CSV
	llmFormatResult, err := client.Extract(&scrapfly.ExtractionConfig{
		ContentType: "text/html",
		Body:        []byte(html),
		// directly request format
		ExtractionPrompt: "get product price and currency in JSON",
	})
	if err != nil {
		log.Fatalf("extraction failed: %v", err)
	}

	fmt.Println("\nllm extraction in JSON:")
	llmFormatResultJSON, _ := json.MarshalIndent(llmFormatResult, "", "  ")
	fmt.Println(string(llmFormatResultJSON))
}

// extractionAutoExtract demonstrates automatic extraction using predefined models
func extractionAutoExtract(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// First, get HTML either from Web Scraping API or your own storage
	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL: "https://web-scraping.dev/product/1",
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}
	html := scrapeResult.Result.Content

	// Auto Extract - extract content using predefined models
	productResult, err := client.Extract(&scrapfly.ExtractionConfig{
		// identify content type like text/html or text/markdown etc.
		ContentType: "text/html",
		Body:        []byte(html),
		// define model type: product, article etc.
		// see https://scrapfly.io/docs/extraction-api/automatic-ai#models
		ExtractionModel: "product",
	})
	if err != nil {
		log.Fatalf("extraction failed: %v", err)
	}

	fmt.Println("product auto extract:")
	productResultJSON, _ := json.MarshalIndent(productResult, "", "  ")
	fmt.Println(string(productResultJSON))
}

// extractionTemplates demonstrates using extraction templates
func extractionTemplates(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// First, get HTML either from Web Scraping API or your own storage
	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL:             "https://web-scraping.dev/reviews",
		RenderJS:        true,
		WaitForSelector: ".review",
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}
	html := scrapeResult.Result.Content

	// Define your template, see https://scrapfly.io/docs/extraction-api/rules-and-template
	template := map[string]interface{}{
		"source": "html",
		"selectors": []map[string]interface{}{
			{
				"name":     "date_posted",
				"type":     "css",
				"query":    "[data-testid='review-date']::text",
				"multiple": true,
				"formatters": []map[string]interface{}{
					{
						"name": "datetime",
						"args": map[string]interface{}{
							"format": "%Y, %b %d — %A",
						},
					},
				},
			},
		},
	}

	templateResult, err := client.Extract(&scrapfly.ExtractionConfig{
		Body:        []byte(html),
		ContentType: "text/html",
		// provide template:
		ExtractionEphemeralTemplate: template,
	})
	if err != nil {
		log.Fatalf("extraction failed: %v", err)
	}

	fmt.Println("template extract:")
	templateResultJSON, _ := json.MarshalIndent(templateResult, "", "  ")
	fmt.Println(string(templateResultJSON))
}

// screenshot demonstrates capturing screenshots
func screenshot(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	screenshotResult, err := client.Screenshot(&scrapfly.ScreenshotConfig{
		URL: "https://web-scraping.dev/product/1",
		// by default 1920x1080 will be captured but resolution can take any value
		Resolution: "540x1200", // for example - tall smartphone viewport
		// to capture all visible parts use capture with full page
		Capture: "fullpage",

		// you can also capture specific elements with css or xpath
		// WaitForSelector: "#reviews", // wait for review to load
		// Capture: "#reviews",  // capture only reviews element

		// for pages that require scrolling to load elements (like endless paging) use
		AutoScroll: true,

		// Simulate vision deficiency (new)
		VisionDeficiencyType: scrapfly.VisionDeficiencyTypeBlurredVision,
	})
	if err != nil {
		log.Fatalf("screenshot failed: %v", err)
	}

	fmt.Println("captured screenshot:")
	fmt.Printf("Format: %s, Size: %d bytes\n", screenshotResult.Metadata.ExtensionName, len(screenshotResult.Image))

	// use the shortcut to save screenshots to file:
	filePath, err := screenshotResult.Save("screenshot")
	if err != nil {
		log.Fatalf("failed to save screenshot: %v", err)
	}
	fmt.Printf("saved screenshot to %s\n", filePath)
}

// downloadFile demonstrates downloading files
func downloadFile(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Build a JavaScript scenario using the scenario builder
	scenario, err := js_scenario.New().
		Click("button[type='submit']").
		Wait(2000).
		Build()
	if err != nil {
		log.Fatalf("failed to build scenario: %v", err)
	}

	scrapeResult, err := client.Scrape(&scrapfly.ScrapeConfig{
		URL: "https://web-scraping.dev/file-download",
		// enable browsers:
		RenderJS: true,
		// this enables more options
		// you can wait for some element to appear:
		WaitForSelector: "#download-btn",
		// you can wait explicitly for N seconds
		RenderingWait: 3000, // 3 seconds
		// you can control the browser through scenarios:
		// https://scrapfly.io/docs/scrape-api/javascript-scenario
		JSScenario: scenario,
		// or even run any custom JS code!
	})
	if err != nil {
		log.Fatalf("scrape failed: %v", err)
	}

	fmt.Println("attachments:")
	attachmentsJSON, _ := json.MarshalIndent(scrapeResult.Result.BrowserData.Attachments, "", "  ")
	fmt.Println(string(attachmentsJSON))

	// use the shortcut to save attachments to file:
	paths, err := scrapeResult.SaveAttachments("./tests_output")
	if err != nil {
		log.Fatalf("failed to save attachments: %v", err)
	}
	for _, path := range paths {
		fmt.Printf("Attachment saved to: %s\n", path)
	}
}

// crawlerQuickstart demonstrates the high-level Crawler API workflow:
// schedule a small crawl, wait for it to finish, read back the seed URL's
// markdown content, and download the WARC artifact.
//
// Run with:
//
//	go run main.go crawlerQuickstart $SCRAPFLY_API_KEY
func crawlerQuickstart(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	crawl := scrapfly.NewCrawl(client, &scrapfly.CrawlerConfig{
		URL:            "https://web-scraping.dev/products",
		PageLimit:      5,
		MaxDuration:    60,
		ContentFormats: []scrapfly.CrawlerContentFormat{scrapfly.CrawlerFormatMarkdown},
	})

	fmt.Println("🕷  scheduling crawl...")
	if err := crawl.Start(); err != nil {
		log.Fatalf("Start failed: %v", err)
	}
	fmt.Printf("   uuid=%s\n", crawl.UUID())

	fmt.Println("⏳ waiting for completion...")
	if err := crawl.Wait(&scrapfly.WaitOptions{
		PollInterval: 2 * time.Second,
		MaxWait:      120 * time.Second,
		Verbose:      true,
	}); err != nil {
		log.Fatalf("Wait failed: %v", err)
	}

	status, _ := crawl.Status(false)
	fmt.Println("✅ crawl finished")
	fmt.Printf("   status=%s is_success=%v\n", status.Status, *status.IsSuccess)
	fmt.Printf("   visited=%d/%d duration=%ds credits=%d\n",
		status.State.URLsVisited, status.State.URLsExtracted,
		status.State.Duration, status.State.APICreditUsed)
	if status.State.StopReason != nil {
		fmt.Printf("   stop_reason=%s\n", *status.State.StopReason)
	}

	fmt.Println("\n📄 reading the seed URL as markdown (plain mode)...")
	md, err := crawl.ReadString("https://web-scraping.dev/products", scrapfly.CrawlerFormatMarkdown)
	if err != nil {
		log.Fatalf("Read failed: %v", err)
	}
	fmt.Printf("   markdown length=%d chars\n", len(md))
	preview := md
	if len(preview) > 200 {
		preview = preview[:200]
	}
	fmt.Printf("   first 200 chars: %s\n", preview)

	fmt.Println("\n📦 downloading WARC artifact...")
	warc, err := crawl.WARC()
	if err != nil {
		log.Fatalf("WARC failed: %v", err)
	}
	fmt.Printf("   warc bytes=%d\n", warc.Len())
	fmt.Println("   (use a library like nlnwa/gowarc to parse the records)")

	fmt.Println("\n🎉 done")
}

// monitoringMetrics demonstrates the Monitoring API (Enterprise plan only).
// See https://scrapfly.io/docs/monitoring#api
func monitoringMetrics(apiKey string) {
	client, err := scrapfly.New(apiKey)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	accountStats, err := client.GetMonitoringMetrics(scrapfly.MonitoringMetricsOptions{
		Aggregation: []scrapfly.MonitoringAggregation{scrapfly.MonitoringAggregationAccount},
		Period:      scrapfly.MonitoringPeriodLast24h,
	})
	if err != nil {
		log.Fatalf("failed to get monitoring metrics: %v", err)
	}
	fmt.Println("==== Account Metrics ====")
	accountJSON, _ := json.MarshalIndent(accountStats["account_metrics"], "", "  ")
	fmt.Println(string(accountJSON))

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	targetStats, err := client.GetMonitoringTargetMetrics(scrapfly.MonitoringTargetMetricsOptions{
		Domain: "httpbin.dev",
		Start:  start,
		End:    end,
	})
	if err != nil {
		log.Fatalf("failed to get target metrics: %v", err)
	}
	fmt.Println("==== Target Metrics on httpbin.dev ====")
	targetJSON, _ := json.MarshalIndent(targetStats, "", "  ")
	fmt.Println(string(targetJSON))
}

func main() {
	// You can enable debug logs to see more details
	scrapfly.DefaultLogger.SetLevel(scrapfly.LevelDebug)

	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <functionName> <apiKey>")
		fmt.Println("Available functions:")
		fmt.Println("  getAccount            - Get account information")
		fmt.Println("  basicGet              - Basic scrape")
		fmt.Println("  jsRender              - Scrape with JS rendering")
		fmt.Println("  scrapeExtraction      - Scrape with inline extraction")
		fmt.Println("  extractionLLM         - Extract content using LLM queries")
		fmt.Println("  extractionAutoExtract - Extract common web objects using Auto Extract")
		fmt.Println("  extractionTemplates   - Extract content using Template engine")
		fmt.Println("  screenshot            - Capture screenshots using Screenshot API")
		fmt.Println("  downloadFile          - Download files using Browser Data Capture")
		fmt.Println("  crawlerQuickstart     - Schedule a small crawl, read content, and download WARC")
		fmt.Println("  monitoringMetrics     - Query Monitoring API aggregates (Enterprise plan)")
		return
	}

	functionName := os.Args[1]
	apiKey := os.Args[2]

	// Map function names to functions
	functions := map[string]func(string){
		"getAccount":            getAccount,
		"basicGet":              basicGet,
		"jsRender":              jsRender,
		"scrapeExtraction":      scrapeExtraction,
		"extractionLLM":         extractionLLM,
		"extractionAutoExtract": extractionAutoExtract,
		"extractionTemplates":   extractionTemplates,
		"screenshot":            screenshot,
		"downloadFile":          downloadFile,
		"crawlerQuickstart":     crawlerQuickstart,
		"monitoringMetrics":     monitoringMetrics,
	}

	fn, exists := functions[functionName]
	if !exists {
		log.Fatalf("Function %s not found", functionName)
	}

	// Execute the function
	fn(apiKey)
}
