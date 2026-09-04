package main

import (
	"fmt"
	"log"
	"os"

	scrapfly "github.com/scrapfly/go-scrapfly"
)

func main() {
	// API key is read from the SCRAPFLY_KEY environment variable.
	client, err := scrapfly.New(os.Getenv("SCRAPFLY_KEY"))
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	// Step 1: Bypass anti-bot protection on the target page.
	// The /unblock endpoint runs Scrapfly's unblocker shields and returns a
	// WebSocket URL pointing at a Cloud Browser session that already has
	// the cleared cookies / state pre-loaded.
	fmt.Println("Calling /unblock...")
	result, err := client.CloudBrowserUnblock(scrapfly.UnblockConfig{
		URL: "https://web-scraping.dev/products",
	})
	if err != nil {
		log.Fatalf("unblock failed: %v", err)
	}

	fmt.Printf("Unblock successful!\n")
	fmt.Printf("  Session: %s\n", result.SessionID)
	fmt.Printf("  WS URL:  %s\n", result.WSURL)

	// Step 2: Connect with a CDP client (chromedp, rod, or playwright-go).
	// chromedp needs NoModifyURL — ws_url is already the CDP endpoint, and the
	// default allocator tries to rediscover one over HTTP and fails on the
	// missing port:
	//
	//   allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, result.WSURL, chromedp.NoModifyURL)
	//   defer cancel()
	//   taskCtx, cancel := chromedp.NewContext(allocCtx)
	//   defer cancel()
	//   var title string
	//   chromedp.Run(taskCtx,
	//       chromedp.Navigate("https://web-scraping.dev/products"),
	//       chromedp.Title(&title))
	//   fmt.Println("Page title:", title)

	fmt.Println("\nConnect to the browser using the WS URL above with chromedp, rod, or playwright-go")
}
