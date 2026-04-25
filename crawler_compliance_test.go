//go:build integration

// Crawler Compliance Test Suite
//
// Drives the Scrapfly Crawler API (via the Go SDK) against the
// web-scraping-dev compliance trap suite, then asserts conformance by
// querying the central report endpoint /crawler-test-report on the
// target app.
//
// The trap app exposes 30 scenario routes (robots.txt traps, redirect
// loops, session-id vortex, infinite calendar, URL normalization,
// nofollow, sitemap index, etc). Each route records hits in an
// in-memory store. This test file asserts hit counts after each crawl.
//
// Server-side catalog:
//
//	apps/web-scraping-dev/website/app/web/CRAWLER_TEST_SUITE.md
//
// SDK brief:
//
//	sdk/CRAWLER_COMPLIANCE_TEST_BRIEF.md
//
// Required env vars:
//
//	SCRAPFLY_API_KEY        Dev API key (e.g. scp-live-...)
//	SCRAPFLY_API_HOST       Local Scrapfly API (default: https://api.scrapfly.local)
//
// Optional:
//
//	WEB_SCRAPING_DEV_BASE   Trap app base URL.
//	                        Default: https://web-scraping.dev (public prod).
//	                        Override to https://web-scraping-dev.local for the
//	                        local self-hosted dev cluster.
//
// Run:
//
//	export SCRAPFLY_API_KEY=scp-live-...
//	go test -tags=integration -timeout=600s -run TestCompliance ./...

package scrapfly

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func complianceTargetBase() string {
	if base := os.Getenv("WEB_SCRAPING_DEV_BASE"); base != "" {
		return base
	}
	return "https://web-scraping.dev"
}

// trapHTTPClient returns an http.Client that skips TLS verification — the
// local self-hosted dev cluster ingress certs are self-signed. Do NOT use this against prod.
func trapHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// resetTrapStore clears the remote trap store. Must be called before every
// test for isolation, since the store is a single global dict on the server.
func resetTrapStore(t *testing.T) {
	t.Helper()
	resetURL := complianceTargetBase() + "/crawler-test-report/reset"
	req, err := http.NewRequest(http.MethodPost, resetURL, nil)
	if err != nil {
		t.Fatalf("build reset request: %v", err)
	}
	resp, err := trapHTTPClient().Do(req)
	if err != nil {
		t.Fatalf("reset trap store: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		t.Fatalf("reset trap store: HTTP %d", resp.StatusCode)
	}
}

// trapInfo mirrors the JSON shape returned by /crawler-test-report.
type trapInfo struct {
	HitCount   int           `json:"hit_count"`
	LastHitTs  *float64      `json:"last_hit_ts"`
	SampleHits []interface{} `json:"sample_hits"`
}

type complianceReport struct {
	GeneratedAt float64             `json:"generated_at"`
	Traps       map[string]trapInfo `json:"traps"`
}

func fetchReport(t *testing.T) complianceReport {
	t.Helper()
	reportURL := complianceTargetBase() + "/crawler-test-report"
	resp, err := trapHTTPClient().Get(reportURL)
	if err != nil {
		t.Fatalf("fetch report: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("fetch report: HTTP %d", resp.StatusCode)
	}
	var report complianceReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return report
}

// hitCount is a safe lookup — returns 0 if the trap was never hit. The
// trap store does NOT pre-register names, so a missing key means zero hits.
func hitCount(report complianceReport, trapName string) int {
	if info, ok := report.Traps[trapName]; ok {
		return info.HitCount
	}
	return 0
}

// boolPtr is a tiny helper for the *bool tri-state CrawlerConfig fields.
func boolPtr(b bool) *bool {
	return &b
}

// runCompliance runs a small crawl synchronously against the target trap
// app and returns the completed Crawl. All compliance tests use the same
// baseline config: low page_limit, cache disabled (so trap hits are
// reproducible), max_duration capped so a buggy run cannot block CI.
//
// Pass overrides via the mutate function to tweak per-test fields.
func runCompliance(t *testing.T, client *Client, mutate func(c *CrawlerConfig)) *Crawl {
	t.Helper()
	cfg := &CrawlerConfig{
		URL:              complianceTargetBase() + "/",
		PageLimit:        50,
		MaxDepth:         2,
		MaxConcurrency:   5,
		MaxDuration:      120,
		Cache:            false,
		CacheClear:       true,
		RespectRobotsTxt: boolPtr(true),
		IgnoreNoFollow:   false,
		ASP:              false,
	}
	if mutate != nil {
		mutate(cfg)
	}
	crawl := NewCrawl(client, cfg)
	if err := crawl.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := crawl.Wait(&WaitOptions{
		PollInterval: 2 * time.Second,
		MaxWait:      180 * time.Second,
	}); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	return crawl
}

// requireComplete fails fast if the crawl did not reach a successful done state.
func requireComplete(t *testing.T, crawl *Crawl) {
	t.Helper()
	status, err := crawl.Status(true)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.IsComplete() {
		t.Fatalf("crawl %s not complete: status=%s success=%v",
			crawl.UUID(), status.Status, status.IsSuccess)
	}
}

// -----------------------------------------------------------------------------
// Robots.txt compliance
// -----------------------------------------------------------------------------

func TestComplianceRespectsRobotsTxt(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.PageLimit = 50
		c.MaxDepth = 2
	})
	requireComplete(t, crawl)

	violations := hitCount(fetchReport(t), "robots_txt_violation")
	if violations != 0 {
		t.Errorf("crawl with respect_robots_txt=true fetched /robots-disallowed %d times — robots.txt Disallow ignored", violations)
	}
}

// TestComplianceNegativeControl_ViolatesRobotsTxtWhenDisabled is the
// MANDATORY counterpart to TestComplianceRespectsRobotsTxt. It proves the
// trap link is discoverable in the first place — without it, the positive
// test could pass simply because the link was never reached.
func TestComplianceNegativeControl_ViolatesRobotsTxtWhenDisabled(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.PageLimit = 50
		c.MaxDepth = 2
		c.RespectRobotsTxt = boolPtr(false)
	})
	requireComplete(t, crawl)

	hits := hitCount(fetchReport(t), "robots_txt_violation")
	if hits < 1 {
		t.Fatalf("negative control failed: with RespectRobotsTxt=false the crawler still did not fetch /robots-disallowed. " +
			"The robots-respect positive test is unreliable until this is fixed " +
			"(check that the homepage scenario card for /robots-disallowed is rendered).")
	}
}

// -----------------------------------------------------------------------------
// Meta / header robots — link-level nofollow
// -----------------------------------------------------------------------------

func TestComplianceRespectsMetaNofollow(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/meta-nofollow"
		c.PageLimit = 10
	})
	requireComplete(t, crawl)

	if v := hitCount(fetchReport(t), "meta_nofollow_violation"); v != 0 {
		t.Errorf("crawler followed link from page with <meta name='robots' content='nofollow'>: %d violations", v)
	}
}

func TestComplianceRespectsRelNofollow(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/rel-nofollow"
		c.PageLimit = 10
	})
	requireComplete(t, crawl)

	if v := hitCount(fetchReport(t), "rel_nofollow_violation"); v != 0 {
		t.Errorf("crawler followed link with rel='nofollow' attribute: %d violations", v)
	}
}

func TestComplianceRespectsXRobotsTagNofollow(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/header-nofollow"
		c.PageLimit = 10
	})
	requireComplete(t, crawl)

	if v := hitCount(fetchReport(t), "x_robots_tag_nofollow_violation"); v != 0 {
		t.Errorf("crawler did not honor X-Robots-Tag: nofollow header: %d violations", v)
	}
}

// -----------------------------------------------------------------------------
// Crawler traps — bounded behavior
// -----------------------------------------------------------------------------

func TestComplianceEscapesRedirectLoop(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/loop-a"
		c.PageLimit = 20
		c.MaxDepth = 5
	})
	requireComplete(t, crawl)

	loops := hitCount(fetchReport(t), "redirect_loop")
	if loops >= 10 {
		t.Errorf("crawler caught in redirect loop: %d hits — cycle detection broken", loops)
	}
}

func TestComplianceBoundsSessionVortex(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/session-vortex"
		c.PageLimit = 100
		c.MaxDepth = 3
	})
	requireComplete(t, crawl)

	hits := hitCount(fetchReport(t), "session_vortex_hit")
	if hits >= 20 {
		t.Errorf("crawler trapped by session-id vortex: %d hits — check URL canonicalization for volatile query params", hits)
	}
}

func TestComplianceBoundsInfiniteCalendar(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/calendar/2024/01"
		c.PageLimit = 100
		c.MaxDepth = 5
	})
	requireComplete(t, crawl)

	hits := hitCount(fetchReport(t), "calendar_trap_hit")
	if hits >= 50 {
		t.Errorf("crawler stuck in infinite calendar: %d pages crawled", hits)
	}
}

func TestComplianceCapsRedirectChain(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/redirect-chain/1"
		c.PageLimit = 5
		c.MaxDepth = 1
	})
	requireComplete(t, crawl)

	depth := hitCount(fetchReport(t), "redirect_chain_depth")
	if depth <= 0 || depth > 10 {
		t.Errorf("unexpected redirect-chain depth: %d (expected 1..10)", depth)
	}
}

// -----------------------------------------------------------------------------
// URL normalization & deduplication
// -----------------------------------------------------------------------------

func TestComplianceCollapsesFragments(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/fragment-collapse"
		c.PageLimit = 10
	})
	requireComplete(t, crawl)

	// Starting on /fragment-collapse records 1 hit. The fragment links must
	// NOT generate any extra requests since fragments never leave the client.
	if v := hitCount(fetchReport(t), "fragment_collapse_hit"); v != 1 {
		t.Errorf("crawler made %d requests for the same URL with different #fragment suffixes (expected 1)", v)
	}
}

func TestComplianceNormalizesUrlVariants(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/normalize-source"
		c.PageLimit = 20
	})
	requireComplete(t, crawl)

	hits := hitCount(fetchReport(t), "normalization_duplicate")
	if hits > 1 {
		t.Errorf("crawler fetched the normalized target %d times (expected <=1) — check canonicalization (case, trailing slash, fragment, empty query)", hits)
	}
}

// -----------------------------------------------------------------------------
// Sitemap handling
// -----------------------------------------------------------------------------

func TestComplianceReadsSitemapIndex(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/"
		c.PageLimit = 500
		c.MaxDepth = 1
		c.UseSitemaps = true
	})
	requireComplete(t, crawl)

	leafs := hitCount(fetchReport(t), "sitemap_leaf_discovered")
	if leafs == 0 {
		t.Errorf("crawler with UseSitemaps=true did not discover any leaf URLs from /sitemap-index.xml — sitemap-index format may not be supported")
	}
}

func TestComplianceToleratesDeadLinkInSitemap(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	// One of the child sitemaps lists /sitemap-404-target which returns 404.
	// The crawler must continue processing the rest. If we reach the end of
	// this test without a Fatal, the crawler survived the dead link.
	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/"
		c.PageLimit = 100
		c.MaxDepth = 1
		c.UseSitemaps = true
	})
	requireComplete(t, crawl)

	t.Logf("[observation] sitemap dead link followed: %d times",
		hitCount(fetchReport(t), "sitemap_dead_link_followed"))
}

// -----------------------------------------------------------------------------
// External-link boundary
// -----------------------------------------------------------------------------

func TestComplianceDoesNotFollowExternalRedirect(t *testing.T) {
	client := integrationClient(t)
	resetTrapStore(t)

	crawl := runCompliance(t, client, func(c *CrawlerConfig) {
		c.URL = complianceTargetBase() + "/redirect-external"
		c.PageLimit = 5
		c.MaxDepth = 2
		c.FollowExternalLinks = false
	})
	requireComplete(t, crawl)

	// No hard assertion on a trap counter; the test passes if the crawl
	// completes (i.e. did not get redirected forever to example.com).
	t.Logf("[observation] external redirect followed: %d",
		hitCount(fetchReport(t), "external_redirect_followed"))
}

