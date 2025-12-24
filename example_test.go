package scrapfly_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/scrapfly/go-scrapfly"
	js_scenario "github.com/scrapfly/go-scrapfly/scenario"
)

func getApiKey() string {
	apiKey := os.Getenv("SCRAPFLY_API_KEY")
	if apiKey == "" {
		log.Fatalf("SCRAPFLY_API_KEY environment variable is not set")
	}
	return apiKey
}

// getAccount demonstrates fetching account information
func Example_getAccount() {
	apiKey := getApiKey()
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
	// Output: Account:
	// {
	// 	"account": {
	// 	  "account_id": "XX-XXX-4c01-a9X97-XXXX",
	// 	  "currency": "USD",
	// 	  "timezone": "Europe/Paris",
	// 	  "suspended": false,
	// 	  "suspension_reason": ""
	// 	},
	// 	"project": {
	// 	  "allow_extra_usage": true,
	// 	  "allowed_networks": [],
	// 	  "budget_limit": null,
	// 	  "budget_spent": null,
	// 	  "concurrency_limit": null,
	// 	  "name": "default",
	// 	  "quota_reached": false,
	// 	  "scrape_request_count": 307,
	// 	  "scrape_request_limit": null,
	// 	  "tags": []
	// 	},
	// 	"subscription": {
	// 	  "billing": {
	// 		"current_extra_scrape_request_price": {
	// 		  "currency": "USD",
	// 		  "amount": 0
	// 		},
	// 		"extra_scrape_request_price_per_10k": {
	// 		  "currency": "USD",
	// 		  "amount": 0
	// 		},
	// 		"ongoing_payment": {
	// 		  "currency": "USD",
	// 		  "amount": 0
	// 		},
	// 		"plan_price": {
	// 		  "currency": "USD",
	// 		  "amount": 0
	// 		}
	// 	  },
	// 	  "extra_scrape_allowed": false,
	// 	  "max_concurrency": 5,
	// 	  "period": {
	// 		"start": "2025-08-01 00:06:34",
	// 		"end": "2025-09-01 00:06:34"
	// 	  },
	// 	  "plan_name": "FREE",
	// 	  "usage": {
	// 		"spider": {
	// 		  "current": 0,
	// 		  "limit": 1
	// 		},
	// 		"schedule": {
	// 		  "current": 0,
	// 		  "limit": 1
	// 		},
	// 		"scrape": {
	// 		  "concurrent_limit": 5,
	// 		  "concurrent_remaining": 5,
	// 		  "concurrent_usage": 0,
	// 		  "current": 307,
	// 		  "extra": 0,
	// 		  "limit": 1000,
	// 		  "remaining": 693
	// 		}
	// 	  }
	// 	}
	//   }

}

// basicGet demonstrates basic scraping with cache and ASP
func Example_basicGet() {
	apiKey := getApiKey()
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
	// Output: scrapfly: 2025/10/26 05:39:16 [DEBUG] scraping url https://httpbin.dev/html
	// scrapfly: 2025/10/26 05:39:19 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/01K8FGECZ0FDFSQ3W11V0M1MMG
	// web log url:
	// https://scrapfly.io/dashboard/monitoring/log/01K8FGECZ0FDFSQ3W11V0M1MMG
	// page content:
	// <!DOCTYPE html>
	// <html>
	// <head>
	// </head>
	// <body>
	//     <h1>Herman Melville - Moby-Dick</h1>
	//     <div>
	//         <p>
	//             Availing himself of the mild, summer-cool weather that now reigned in these latitudes, and in preparation
	//             for the peculiarly active pursuits shortly to be anticipated, Perth, the begrimed, blistered old blacksmith,
	//             had not removed his portable forge to the hold again, after concluding his contributory work for Ahab's leg,
	//             but still retained it on deck, fast lashed to ringbolts by the foremast; being now almost incessantly
	//             invoked by the headsmen, and harpooneers, and bowsmen to do some little job for them; altering, or
	//             repairing, or new shaping their various weapons and boat furniture. Often he would be surrounded by an eager
	//             circle, all waiting to be served; holding boat-spades, pike-heads, harpoons, and lances, and jealously
	//             watching his every sooty movement, as he toiled. Nevertheless, this old man's was a patient hammer wielded
	//             by a patient arm. No murmur, no impatience, no petulance did come from him. Silent, slow, and solemn; bowing
	//             over still further his chronically broken back, he toiled away, as if toil were life itself, and the heavy
	//             beating of his hammer the heavy beating of his heart. And so it was.—Most miserable! A peculiar walk in this
	//             old man, a certain slight but painful appearing yawing in his gait, had at an early period of the voyage
	//             excited the curiosity of the mariners. And to the importunity of their persisted questionings he had finally
	//             given in; and so it came to pass that every one now knew the shameful story of his wretched fate. Belated,
	//             and not innocently, one bitter winter's midnight, on the road running between two country towns, the
	//             blacksmith half-stupidly felt the deadly numbness stealing over him, and sought refuge in a leaning,
	//             dilapidated barn. The issue was, the loss of the extremities of both feet. Out of this revelation, part by
	//             part, at last came out the four acts of the gladness, and the one long, and as yet uncatastrophied fifth act
	//             of the grief of his life's drama. He was an old man, who, at the age of nearly sixty, had postponedly
	//             encountered that thing in sorrow's technicals called ruin. He had been an artisan of famed excellence, and
	//             with plenty to do; owned a house and garden; embraced a youthful, daughter-like, loving wife, and three
	//             blithe, ruddy children; every Sunday went to a cheerful-looking church, planted in a grove. But one night,
	//             under cover of darkness, and further concealed in a most cunning disguisement, a desperate burglar slid into
	//             his happy home, and robbed them all of everything. And darker yet to tell, the blacksmith himself did
	//             ignorantly conduct this burglar into his family's heart. It was the Bottle Conjuror! Upon the opening of
	//             that fatal cork, forth flew the fiend, and shrivelled up his home. Now, for prudent, most wise, and economic
	//             reasons, the blacksmith's shop was in the basement of his dwelling, but with a separate entrance to it; so
	//             that always had the young and loving healthy wife listened with no unhappy nervousness, but with vigorous
	//             pleasure, to the stout ringing of her young-armed old husband's hammer; whose reverberations, muffled by
	//             passing through the floors and walls, came up to her, not unsweetly, in her nursery; and so, to stout
	//             Labor's iron lullaby, the blacksmith's infants were rocked to slumber. Oh, woe on woe! Oh, Death, why canst
	//             thou not sometimes be timely? Hadst thou taken this old blacksmith to thyself ere his full ruin came upon
	//             him, then had the young widow had a delicious grief, and her orphans a truly venerable, legendary sire to
	//             dream of in their after years; and all of them a care-killing competency.
	//         </p>
	//     </div>
	//     <hr>
	//     <h2>Heading Level 2</h2>
	//     <h3>Heading Level 3</h3>
	//     <p>This is a paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
	//     <blockquote>
	//         <p>This is a blockquote for markdown testing.</p>
	//     </blockquote>
	//     <pre><code>// This is a code block
	// function test() {
	//   return true;
	// }
	// </code></pre>
	//     <ul>
	//         <li>Unordered list item 1
	//             <ul>
	//                 <li>Nested unordered item A</li>
	//                 <li>Nested unordered item B</li>
	//             </ul>
	//         </li>
	//         <li>Unordered list item 2</li>
	//     </ul>
	//     <ol>
	//         <li>Ordered list item 1
	//             <ol>
	//                 <li>Nested ordered item A</li>
	//                 <li>Nested ordered item B</li>
	//             </ol>
	//         </li>
	//         <li>Ordered list item 2</li>
	//     </ol>
	//     <p>Link: <a href="https://httpbin.dev">httpbin.dev</a></p>
	//     <p>Image: <img src="https://httpbin.dev/image/png" alt="Test Image" width="100"></p>
	//     <h3>Vertical Table</h3>
	//     <table>
	//         <tr>
	//             <th>Header 1</th>
	//             <th>Header 2</th>
	//         </tr>
	//         <tr>
	//             <td>Cell 1</td>
	//             <td>Cell 2</td>
	//         </tr>xwx
	//         <tr>
	//             <td>Cell 3</td>
	//             <td>Cell 4</td>
	//         </tr>
	//     </table>
	//     <h3>Horizontal Table</h3>
	//     <table border="1">
	//         <tr>
	//             <th>Name</th>
	//             <th>Age</th>
	//             <th>Country</th>
	//         </tr>
	//         <tr>
	//             <td>Alice</td>
	//             <td>30</td>
	//             <td>USA</td>
	//         </tr>
	//         <tr>
	//             <td>Bob</td>
	//             <td>25</td>
	//             <td>UK</td>
	//         </tr>
	//         <tr>
	//             <td>Charlie</td>
	//             <td>35</td>
	//             <td>Canada</td>
	//         </tr>
	//     </table>
	//     <h3>Vertical Table</h3>
	//     <table border="1">
	//         <tr>
	//             <th>Field</th>
	//             <th>Value</th>
	//         </tr>
	//         <tr>
	//             <td>Name</td>
	//             <td>Alice</td>
	//         </tr>
	//         <tr>
	//             <td>Age</td>
	//             <td>30</td>
	//         </tr>
	//         <tr>
	//             <td>Country</td>
	//             <td>USA</td>
	//         </tr>
	//     </table>
	// </body>
	//
	// </html>
	//
	// response headers:
	// {
	//   "access-control-allow-credentials": "true",
	//   "access-control-allow-origin": "*",
	//   "alt-svc": "h3=\":443\"; ma=2592000",
	//   "content-security-policy": "frame-ancestors 'self' *.httpbin.dev; font-src 'self' *.httpbin.dev; default-src 'self' *.httpbin.dev; img-src 'self' *.httpbin.dev https://cdn.scrapfly.io; media-src 'self' *.httpbin.dev; object-src 'self' *.httpbin.dev https://web-scraping.dev; script-src 'self' 'unsafe-inline' 'unsafe-eval' *.httpbin.dev; style-src 'self' 'unsafe-inline' *.httpbin.dev https://unpkg.com; frame-src 'self' *.httpbin.dev https://web-scraping.dev; worker-src 'self' *.httpbin.dev; connect-src 'self' *.httpbin.dev",
	//   "content-type": "text/html; charset=utf-8",
	//   "date": "Sun, 26 Oct 2025 05:39:19 GMT",
	//   "permissions-policy": "fullscreen=(self), autoplay=*, geolocation=(), camera=()",
	//   "referrer-policy": "strict-origin-when-cross-origin",
	//   "strict-transport-security": "max-age=31536000; includeSubDomains; preload",
	//   "x-content-type-options": "nosniff",
	//   "x-xss-protection": "1; mode=block"
	// }
	//
	// response cookies:
	// []

}

func Example_downloadFile() {
	apiKey := getApiKey()
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
	// Output: scrapfly: 2025/11/06 18:30:41 [DEBUG] scraping url https://web-scraping.dev/file-download
	// scrapfly: 2025/11/06 18:30:55 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
	// attachments:
	// [
	//   {
	//     "content": "https://api.scrapfly.io/scrape/attachment/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX/7024607f-f336-4a79-9841-5fc2bb06c41e",
	//     "content_type": "application/pdf",
	//     "filename": "download-sample.pdf",
	//     "id": "7024607f-f336-4a79-9841-5fc2bb06c41e",
	//     "size": 10360,
	//     "state": "completed",
	//     "suggested_filename": "download-sample.pdf",
	//     "url": "https://web-scraping.dev/api/download-file"
	//   }
	// ]
	// Attachment saved to: tests_output/download-sample.pdf
}

// jsRender demonstrates JavaScript rendering with scenarios
func Example_jsRender() {
	apiKey := getApiKey()
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
	// Output: scrapfly: 2025/10/26 05:39:19 [DEBUG] scraping url https://web-scraping.dev/product/1
	// scrapfly: 2025/10/26 05:39:34 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/01K8FGEG1NB3RJ183CJ27YD25N
	// web log url:
	// https://scrapfly.io/dashboard/monitoring/log/01K8FGEG1NB3RJ183CJ27YD25N
	//
	// page content (first 1000 chars):
	// <html lang="en"><head>
	//     <meta charset="utf-8">
	//     <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
	//     <link rel="stylesheet" href="https://web-scraping.dev/assets/css/main.css">
	//     <link rel="stylesheet" href="https://web-scraping.dev/assets/css/bootstrap.min.css">
	//     <link rel="stylesheet" href="https://web-scraping.dev/assets/css/bootstrap-icons.css">
	//     <link rel="stylesheet" href="https://web-scraping.dev/assets/css/highlight-nord.css">
	//     <link rel="icon" href="https://web-scraping.dev/assets/media/icon.png" type="image/png">
	//     <script src="https://web-scraping.dev/assets/js/cash.min.js"></script>
	//     <script src="https://web-scraping.dev/assets/js/main.js"></script>
	//     <script src="https://web-scraping.dev/assets/js/bootstrap.js"></script>
	// <title>web-scraping.dev product Box of Chocolate Candy</title>
	// <meta name="description" content="Mock product Box of Chocolate Candy page for web scraper testing">
	// <meta property="og:sit...
	// browser data capture:
	//
	//	{
	//	  "javascript_evaluation_result": "2022-07-22     \n\nAbsolutely delicious! The orange flavor is my favorite.",
	//	  "js_scenario": {
	//	    "duration": 3.0700000000000003,
	//	    "executed": 2,
	//	    "response": null,
	//	    "steps": [
	//	      {
	//	        "action": "click",
	//	        "config": {
	//	          "ignore_if_not_visible": false,
	//	          "multiple": false,
	//	          "selector": "#load-more-reviews",
	//	          "timeout": 3500
	//	        },
	//	        "duration": 1.07,
	//	        "executed": true,
	//	        "result": null,
	//	        "success": true
	//	      },
	//	      {
	//	        "action": "wait",
	//	        "config": 2000,
	//	        "duration": 2,
	//	        "executed": true,
	//	        "result": null,
	//	        "success": true
	//	      }
	//	    ]
	//	  },
	//	  "local_storage_data": {},
	//	  "session_storage_data": {},
	//	  "websockets": [],
	//	  "xhr_call": [
	//	    {
	//	      "body": null,
	//	      "headers": {
	//	        "Accept": "* /*",
	//	        "Referer": "https://web-scraping.dev/product/1",
	//	        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36",
	//	        "sec-ch-ua": "\"Google Chrome\";v=\"141\", \"Not?A_Brand\";v=\"8\", \"Chromium\";v=\"141\"",
	//	        "sec-ch-ua-mobile": "?0",
	//	        "sec-ch-ua-platform": "\"macOS\"",
	//	        "x-csrf-token": "secret-csrf-token-123"
	//	      },
	//	      "method": "GET",
	//	      "response": {
	//	        "body": "{\"order\":\"asc\",\"category\":null,\"total_results\":10,\"next_url\":null,\"results\":[{\"id\":\"chocolate-candy-box-6\",\"text\":\"Bought the large box, and it's lasted quite a while. Great for when you need a sweet treat.\",\"rating\":5,\"date\":\"2022-12-18\"},{\"id\":\"chocolate-candy-box-7\",\"text\":\"These chocolates are so tasty! Love the variety of flavors.\",\"rating\":5,\"date\":\"2023-01-24\"},{\"id\":\"chocolate-candy-box-8\",\"text\":\"The box is nicely packaged, making it a great gift option.\",\"rating\":5,\"date\":\"2023-02-15\"},{\"id\":\"chocolate-candy-box-9\",\"text\":\"The orange flavor wasn't my favorite, but the cherry ones are great.\",\"rating\":4,\"date\":\"2023-03-20\"},{\"id\":\"chocolate-candy-box-10\",\"text\":\"Delicious chocolates, and the box is pretty substantial. It'd make a nice gift.\",\"rating\":5,\"date\":\"2023-04-18\"}],\"page_number\":2,\"page_size\":5,\"page_total\":2}",
	//	        "content_encoding": null,
	//	        "content_type": "application/json",
	//	        "duration": 0,
	//	        "format": "text",
	//	        "headers": {
	//	          "alt-svc": "h3=\":443\"; ma=2592000",
	//	          "content-length": "839",
	//	          "content-type": "application/json",
	//	          "date": "Sun, 26 Oct 2025 05:39:31 GMT",
	//	          "permissions-policy": "fullscreen=(self), autoplay=*, geolocation=(), camera=()",
	//	          "referrer-policy": "strict-origin-when-cross-origin",
	//	          "server": "uvicorn",
	//	          "strict-transport-security": "max-age=31536000; includeSubDomains; preload",
	//	          "x-content-type-options": "nosniff",
	//	          "x-xss-protection": "1; mode=block"
	//	        },
	//	        "status": 200
	//	      },
	//	      "type": "fetch",
	//	      "url": "https://web-scraping.dev/api/reviews?product_id=1\u0026page=2"
	//	    }
	//	  ]
	//	}
}

// scrapeExtraction demonstrates scraping with inline extraction using LLM prompts
func Example_scrapeExtraction() {
	apiKey := getApiKey()
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
	// Output: scrapfly: 2025/11/07 01:01:46 [DEBUG] scraping url https://web-scraping.dev/product/1
	// scrapfly: 2025/11/07 01:01:58 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/XXXXXXXXXXXXX
	// extraction result:
	// {
	//   "data": {
	//     "product": {
	//       "aggregateRating": {
	//         "ratingValue": "4.7",
	//         "reviewCount": "10"
	//       },
	//       "brand": "ChocoDelight",
	//       "description": "Indulge your sweet tooth with our Box of Chocolate Candy. Each box contains an assortment of rich, flavorful chocolates with a smooth, creamy filling. Choose from a variety of flavors including zesty orange and sweet cherry. Whether you're looking for the perfect gift or just want to treat yourself, our Box of Chocolate Candy is sure to satisfy.",
	//       "features": {
	//         "care instructions": "Store in a cool, dry place",
	//         "flavors": "Available in Orange and Cherry flavors",
	//         "material": "Premium quality chocolate",
	//         "purpose": "Ideal for gifting or self-indulgence",
	//         "sizes": "Available in small, medium, and large boxes"
	//       },
	//       "image": "https://web-scraping.dev/assets/products/orange-chocolate-box-medium-1.webp",
	//       "name": "Box of Chocolate Candy",
	//       "offers": {
	//         "availability": "InStock",
	//         "highPrice": "19.99",
	//         "lowPrice": "9.99",
	//         "priceCurrency": "USD"
	//       },
	//       "packs": [
	//         {
	//           "Delivery Type": "1 Day shipping",
	//           "Package Dimension": "100x230 cm",
	//           "Package Weight": "1,00 kg",
	//           "Variants": "6 available",
	//           "Version": "Pack 1"
	//         },
	//         {
	//           "Delivery Type": "1 Day shipping",
	//           "Package Dimension": "200x460 cm",
	//           "Package Weight": "2,11 kg",
	//           "Variants": "6 available",
	//           "Version": "Pack 2"
	//         },
	//         {
	//           "Delivery Type": "1 Day shipping",
	//           "Package Dimension": "300x690 cm",
	//           "Package Weight": "3,22 kg",
	//           "Variants": "6 available",
	//           "Version": "Pack 3"
	//         },
	//         {
	//           "Delivery Type": "1 Day shipping",
	//           "Package Dimension": "400x920 cm",
	//           "Package Weight": "4,33 kg",
	//           "Variants": "6 available",
	//           "Version": "Pack 4"
	//         },
	//         {
	//           "Delivery Type": "1 Day shipping",
	//           "Package Dimension": "500x1150 cm",
	//           "Package Weight": "5,44 kg",
	//           "Variants": "6 available",
	//           "Version": "Pack 5"
	//         }
	//       ],
	//       "reviews": [
	//         {
	//           "datePublished": "2022-07-22",
	//           "ratingValue": "5",
	//           "reviewBody": "Absolutely delicious! The orange flavor is my favorite."
	//         },
	//         {
	//           "datePublished": "2022-08-16",
	//           "ratingValue": "4",
	//           "reviewBody": "I bought these as a gift, and they were well received. Will definitely purchase again."
	//         },
	//         {
	//           "datePublished": "2022-09-10",
	//           "ratingValue": "5",
	//           "reviewBody": "Nice variety of flavors. The chocolate is rich and smooth."
	//         },
	//         {
	//           "datePublished": "2022-10-02",
	//           "ratingValue": "5",
	//           "reviewBody": "The cherry flavor is amazing. Will be buying more."
	//         },
	//         {
	//           "datePublished": "2022-11-05",
	//           "ratingValue": "4",
	//           "reviewBody": "A bit pricey, but the quality of the chocolate is worth it."
	//         }
	//       ],
	//       "variants": [
	//         "orange, small",
	//         "orange, medium",
	//         "orange, large",
	//         "cherry, small",
	//         "cherry, medium",
	//         "cherry, large"
	//       ]
	//     }
	//   },
	//   "content_type": "application/json"
	// }
}

// extractionLLM demonstrates using the extraction API with LLM prompts
func Example_extractionLLM() {
	apiKey := getApiKey()
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
	// Output: scrapfly: 2025/11/07 01:01:58 [DEBUG] scraping url https://web-scraping.dev/product/1
	// scrapfly: 2025/11/07 01:02:00 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/XXXXXXXXXXXXX
	// llm extraction:
	// {
	//   "data": "The price of the \"Box of Chocolate Candy\" is $9.99 from $12.99. The lowPrice is \"9.99\" and the highPrice is \"19.99\" according to the JSON-LD metadata.",
	//   "content_type": "text/plain"
	// }
	//
	// llm extraction in JSON:
	//
	//	{
	//	  "data": {
	//	    "product": "Box of Chocolate Candy",
	//	    "similar_products": [
	//	      {
	//	        "currency": "USD",
	//	        "price": "4.99",
	//	        "product": "Dragon Energy Potion"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "89.99",
	//	        "product": "Hiking Boots for Outdoor Adventures"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "4.99",
	//	        "product": "Teal Energy Potion"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "14.99",
	//	        "product": "Cat-Ear Beanie"
	//	      }
	//	    ],
	//	    "variants": [
	//	      {
	//	        "currency": "USD",
	//	        "price": "9.99",
	//	        "variant": "orange, small"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "9.99",
	//	        "variant": "orange, medium"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "9.99",
	//	        "variant": "orange, large"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "9.99",
	//	        "variant": "cherry, small"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "9.99",
	//	        "variant": "cherry, medium"
	//	      },
	//	      {
	//	        "currency": "USD",
	//	        "price": "9.99",
	//	        "variant": "cherry, large"
	//	      }
	//	    ]
	//	  },
	//	  "content_type": "application/json"
	//	}
}

// extractionAutoExtract demonstrates automatic extraction using predefined models
func Example_extractionAutoExtract() {
	apiKey := getApiKey()
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
	// Output:scrapfly: 2025/11/17 05:22:31 [DEBUG] scraping url https://web-scraping.dev/product/1
	// scrapfly: 2025/11/17 05:22:35 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/XXXXXX
	// product auto extract:
	// {
	//   "data": {
	//     "aggregate_rating": {
	//       "best_rating": 5,
	//       "rating_value": 4.7,
	//       "review_count": 10
	//     },
	//     "bidding": null,
	//     "brand": "ChocoDelight",
	//     "breadcrumbs": [
	//       {
	//         "link": "/",
	//         "name": "Home"
	//       },
	//       {
	//         "link": "/products",
	//         "name": "Products"
	//       },
	//       {
	//         "link": null,
	//         "name": "Box of Chocolate Candy"
	//       }
	//     ],
	//     "canonical_url": null,
	//     "color": null,
	//     "delivery": "1 Day shipping",
	//     "description": "Indulge your sweet tooth with our Box of Chocolate Candy. Each box contains an assortment of rich, flavorful chocolates with a smooth, creamy filling. Choose from a variety of flavors including zesty orange and sweet cherry. Whether you're looking for the perfect gift or just want to treat yourself, our Box of Chocolate Candy is sure to satisfy.",
	//     "description_markdown": "Indulge your sweet tooth with our Box of Chocolate Candy. Each box contains an assortment of rich, flavorful chocolates with a smooth, creamy filling. Choose from a variety of flavors including zesty orange and sweet cherry. Whether you're looking for the perfect gift or just want to treat yourself, our Box of Chocolate Candy is sure to satisfy.",
	//     "identifiers": {
	//       "ean13": null,
	//       "gtin14": null,
	//       "gtin8": null,
	//       "isbn10": null,
	//       "isbn13": null,
	//       "ismn": null,
	//       "issn": null,
	//       "mpn": null,
	//       "sku": "1",
	//       "upc": null
	//     },
	//     "images": [
	//       {
	//         "url": "https://www.web-scraping.dev/assets/products/orange-chocolate-box-small-1.webp"
	//       },
	//       {
	//         "url": "https://www.web-scraping.dev/assets/products/orange-chocolate-box-small-2.webp"
	//       },
	//       {
	//         "url": "https://www.web-scraping.dev/assets/products/orange-chocolate-box-small-3.webp"
	//       },
	//       {
	//         "url": "https://www.web-scraping.dev/assets/products/orange-chocolate-box-small-4.webp"
	//       }
	//     ],
	//     "main_category": "Products",
	//     "main_image": "https://www.web-scraping.dev/assets/products/orange-chocolate-box-small-1.webp",
	//     "name": "Box of Chocolate Candy",
	//     "offers": [
	//       {
	//         "availability": "available",
	//         "currency": "USD",
	//         "price": 9.99,
	//         "regular_price": 12.99
	//       }
	//     ],
	//     "related_products": [
	//       {
	//         "availability": "available",
	//         "description": null,
	//         "images": [
	//           {
	//             "url": "https://www.web-scraping.dev/assets/products/blue-potion.webp"
	//           }
	//         ],
	//         "link": "https://web-scraping.dev/product/17",
	//         "name": "Blue Energy Potion",
	//         "price": {
	//           "amount": 4.99,
	//           "currency": "USD",
	//           "raw": "4.99"
	//         }
	//       },
	//       {
	//         "availability": "available",
	//         "description": null,
	//         "images": [
	//           {
	//             "url": "https://www.web-scraping.dev/assets/products/teal-potion.webp"
	//           }
	//         ],
	//         "link": "https://web-scraping.dev/product/27",
	//         "name": "Teal Energy Potion",
	//         "price": {
	//           "amount": 4.99,
	//           "currency": "USD",
	//           "raw": "4.99"
	//         }
	//       },
	//       {
	//         "availability": "available",
	//         "description": null,
	//         "images": [
	//           {
	//             "url": "https://www.web-scraping.dev/assets/products/orange-chocolate-box-medium-1.webp"
	//           }
	//         ],
	//         "link": "https://web-scraping.dev/product/25",
	//         "name": "Box of Chocolate Candy",
	//         "price": {
	//           "amount": 24.99,
	//           "currency": "USD",
	//           "raw": "24.99"
	//         }
	//       },
	//       {
	//         "availability": "available",
	//         "description": null,
	//         "images": [
	//           {
	//             "url": "https://www.web-scraping.dev/assets/products/hiking-boots-1.webp"
	//           }
	//         ],
	//         "link": "https://web-scraping.dev/product/7",
	//         "name": "Hiking Boots for Outdoor Adventures",
	//         "price": {
	//           "amount": 89.99,
	//           "currency": "USD",
	//           "raw": "89.99"
	//         }
	//       }
	//     ],
	//     "secondary_category": null,
	//     "size": null,
	//     "sizes": null,
	//     "specifications": [
	//       {
	//         "name": "material",
	//         "value": "Premium quality chocolate"
	//       },
	//       {
	//         "name": "flavors",
	//         "value": "Available in Orange and Cherry flavors"
	//       },
	//       {
	//         "name": "sizes",
	//         "value": "Available in small, medium, and large boxes"
	//       },
	//       {
	//         "name": "brand",
	//         "value": "ChocoDelight"
	//       },
	//       {
	//         "name": "care instructions",
	//         "value": "Store in a cool, dry place"
	//       },
	//       {
	//         "name": "purpose",
	//         "value": "Ideal for gifting or self-indulgence"
	//       }
	//     ],
	//     "style": null,
	//     "url": "https://web-scraping.dev/product/1",
	//     "variants": [
	//       {
	//         "color": "orange",
	//         "offers": [],
	//         "sku": null,
	//         "url": "https://web-scraping.dev/product/1?variant=orange-small"
	//       },
	//       {
	//         "color": "orange",
	//         "offers": [],
	//         "sku": null,
	//         "url": "https://web-scraping.dev/product/1?variant=orange-medium"
	//       },
	//       {
	//         "color": "orange",
	//         "offers": [],
	//         "sku": null,
	//         "url": "https://web-scraping.dev/product/1?variant=orange-large"
	//       },
	//       {
	//         "color": "cherry",
	//         "offers": [],
	//         "sku": null,
	//         "url": "https://web-scraping.dev/product/1?variant=cherry-small"
	//       },
	//       {
	//         "color": "cherry",
	//         "offers": [],
	//         "sku": null,
	//         "url": "https://web-scraping.dev/product/1?variant=cherry-medium"
	//       },
	//       {
	//         "color": "cherry",
	//         "offers": [],
	//         "sku": null,
	//         "url": "https://web-scraping.dev/product/1?variant=cherry-large"
	//       }
	//     ]
	//   },
	//   "content_type": "application/json",
	//   "data_quality": {
	//     "errors": [
	//       "breadcrumbs[2].link: Input should be a valid string",
	//       "breadcrumbs[2].link: Input should be a valid string"
	//     ],
	//     "fulfilled": false,
	//     "fulfillment_percent": 69.57
	//   }
	// }
}

// extractionTemplates demonstrates using extraction templates
func Example_extractionTemplates() {
	apiKey := getApiKey()
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
	// Output: scrapfly: 2025/10/26 05:40:28 [DEBUG] scraping url https://web-scraping.dev/reviews
	// scrapfly: 2025/10/26 05:40:35 [DEBUG] scrape log url: https://scrapfly.io/dashboard/monitoring/log/01K8FGGJX0AY63MHW2ACEY4ZFZ
	// template extract:
	// {
	//   "data": {
	//     "date_posted": [
	//       "2023, May 18 — Thursday",
	//       "2023, May 17 — Wednesday",
	//       "2023, May 16 — Tuesday",
	//       "2023, May 15 — Monday",
	//       "2023, May 15 — Monday",
	//       "2023, May 12 — Friday",
	//       "2023, May 10 — Wednesday",
	//       "2023, May 01 — Monday",
	//       "2023, May 01 — Monday",
	//       "2023, Apr 25 — Tuesday",
	//       "2023, Apr 25 — Tuesday",
	//       "2023, Apr 18 — Tuesday",
	//       "2023, Apr 12 — Wednesday",
	//       "2023, Apr 11 — Tuesday",
	//       "2023, Apr 10 — Monday",
	//       "2023, Apr 10 — Monday",
	//       "2023, Apr 09 — Sunday",
	//       "2023, Apr 07 — Friday",
	//       "2023, Apr 07 — Friday",
	//       "2023, Apr 05 — Wednesday"
	//     ]
	//   },
	//   "content_type": "application/json"
	// }
}

// screenshot demonstrates capturing screenshots
func Example_screenshot() {
	apiKey := getApiKey()

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
	// Output: captured screenshot:
	// Format: jpeg, Size: 586399 bytes
	// saved screenshot to screenshot.jpeg
}
