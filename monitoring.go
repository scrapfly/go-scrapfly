package scrapfly

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MonitoringDataFormat controls the response format of the Monitoring API.
// "structured" returns JSON aggregates (default); "prometheus" returns a
// Prometheus-compatible text exposition.
type MonitoringDataFormat string

const (
	MonitoringDataFormatStructured MonitoringDataFormat = "structured"
	MonitoringDataFormatPrometheus MonitoringDataFormat = "prometheus"
)

// MonitoringPeriod is a pre-defined time window for monitoring queries.
// Mutually exclusive with explicit start/end.
type MonitoringPeriod string

const (
	MonitoringPeriodLast5m       MonitoringPeriod = "last5m"
	MonitoringPeriodLast1h       MonitoringPeriod = "last1h"
	MonitoringPeriodLast24h      MonitoringPeriod = "last24h"
	MonitoringPeriodLast7d       MonitoringPeriod = "last7d"
	MonitoringPeriodSubscription MonitoringPeriod = "subscription"
)

// MonitoringAggregation selects which aggregation level(s) to return for
// /scrape/monitoring/metrics. Multiple values can be combined.
type MonitoringAggregation string

const (
	MonitoringAggregationAccount MonitoringAggregation = "account"
	MonitoringAggregationProject MonitoringAggregation = "project"
	MonitoringAggregationTarget  MonitoringAggregation = "target"
)

// MonitoringMetricsOptions configures a Get*MonitoringMetrics call.
// All fields are optional — an empty struct returns account aggregates
// for the last 24 hours in structured JSON.
type MonitoringMetricsOptions struct {
	Format      MonitoringDataFormat
	Period      MonitoringPeriod
	Aggregation []MonitoringAggregation
	// IncludeWebhook folds events with origin=WEBHOOK (callbacks
	// executed by the webhook worker) into this product's totals.
	// Defaults to false to match the dashboard's default view.
	IncludeWebhook bool
}

// MonitoringTargetMetricsOptions configures a Get*MonitoringTargetMetrics
// call. Domain is required. Start/End are mutually exclusive with Period;
// both must be set together.
type MonitoringTargetMetricsOptions struct {
	Domain         string
	GroupSubdomain bool
	Period         MonitoringPeriod
	Start          time.Time
	End            time.Time
	IncludeWebhook bool
}

// CloudBrowserMonitoringOptions configures Cloud Browser monitoring
// queries. Cloud Browser is session-based with a distinct shape from
// the request-based products (no domain/target, no include_webhook).
type CloudBrowserMonitoringOptions struct {
	Period    MonitoringPeriod
	ProxyPool string
	Start     time.Time
	End       time.Time
}

const monitoringDatetimeFormat = "2006-01-02 15:04:05"

// buildMonitoringMetricsURL builds the URL + query for a request-based
// metrics call, scoped to the given product path (e.g. "/scrape",
// "/screenshot", "/extraction", "/crawl").
func (c *Client) buildMonitoringMetricsURL(productPath string, opts MonitoringMetricsOptions) string {
	endpointURL, _ := url.Parse(c.host + productPath + "/monitoring/metrics")
	params := url.Values{}
	params.Set("key", c.key)
	format := opts.Format
	if format == "" {
		format = MonitoringDataFormatStructured
	}
	params.Set("format", string(format))
	if opts.Period != "" {
		params.Set("period", string(opts.Period))
	}
	if len(opts.Aggregation) > 0 {
		agg := make([]string, len(opts.Aggregation))
		for i, a := range opts.Aggregation {
			agg[i] = string(a)
		}
		params.Set("aggregation", strings.Join(agg, ","))
	}
	if opts.IncludeWebhook {
		params.Set("include_webhook", "true")
	}
	endpointURL.RawQuery = params.Encode()
	return endpointURL.String()
}

// buildMonitoringTargetURL builds the URL + query for a request-based
// per-target call.
func (c *Client) buildMonitoringTargetURL(productPath string, opts MonitoringTargetMetricsOptions) (string, error) {
	if opts.Domain == "" {
		return "", fmt.Errorf("monitoring target metrics: domain is required")
	}
	if (!opts.Start.IsZero()) != (!opts.End.IsZero()) {
		return "", fmt.Errorf("monitoring target metrics: start and end must be provided together")
	}
	endpointURL, _ := url.Parse(c.host + productPath + "/monitoring/metrics/target")
	params := url.Values{}
	params.Set("key", c.key)
	params.Set("domain", opts.Domain)
	params.Set("group_subdomain", strconv.FormatBool(opts.GroupSubdomain))
	if !opts.Start.IsZero() && !opts.End.IsZero() {
		params.Set("start", opts.Start.UTC().Format(monitoringDatetimeFormat))
		params.Set("end", opts.End.UTC().Format(monitoringDatetimeFormat))
	} else if opts.Period != "" {
		params.Set("period", string(opts.Period))
	} else {
		params.Set("period", string(MonitoringPeriodLast24h))
	}
	if opts.IncludeWebhook {
		params.Set("include_webhook", "true")
	}
	endpointURL.RawQuery = params.Encode()
	return endpointURL.String(), nil
}

// ── Web Scraping API (Enterprise+ plan only) ─────────────────────────
// See https://scrapfly.io/docs/monitoring#api

func (c *Client) GetMonitoringMetrics(opts MonitoringMetricsOptions) (map[string]any, error) {
	return c.doMonitoringRequest(c.buildMonitoringMetricsURL("/scrape", opts))
}

func (c *Client) GetMonitoringTargetMetrics(opts MonitoringTargetMetricsOptions) (map[string]any, error) {
	requestURL, err := c.buildMonitoringTargetURL("/scrape", opts)
	if err != nil {
		return nil, err
	}
	return c.doMonitoringRequest(requestURL)
}

// ── Screenshot API ───────────────────────────────────────────────────

func (c *Client) GetScreenshotMonitoringMetrics(opts MonitoringMetricsOptions) (map[string]any, error) {
	return c.doMonitoringRequest(c.buildMonitoringMetricsURL("/screenshot", opts))
}

func (c *Client) GetScreenshotMonitoringTargetMetrics(opts MonitoringTargetMetricsOptions) (map[string]any, error) {
	requestURL, err := c.buildMonitoringTargetURL("/screenshot", opts)
	if err != nil {
		return nil, err
	}
	return c.doMonitoringRequest(requestURL)
}

// ── Extraction API ───────────────────────────────────────────────────

func (c *Client) GetExtractionMonitoringMetrics(opts MonitoringMetricsOptions) (map[string]any, error) {
	return c.doMonitoringRequest(c.buildMonitoringMetricsURL("/extraction", opts))
}

func (c *Client) GetExtractionMonitoringTargetMetrics(opts MonitoringTargetMetricsOptions) (map[string]any, error) {
	requestURL, err := c.buildMonitoringTargetURL("/extraction", opts)
	if err != nil {
		return nil, err
	}
	return c.doMonitoringRequest(requestURL)
}

// ── Crawler API ──────────────────────────────────────────────────────

func (c *Client) GetCrawlerMonitoringMetrics(opts MonitoringMetricsOptions) (map[string]any, error) {
	return c.doMonitoringRequest(c.buildMonitoringMetricsURL("/crawl", opts))
}

func (c *Client) GetCrawlerMonitoringTargetMetrics(opts MonitoringTargetMetricsOptions) (map[string]any, error) {
	requestURL, err := c.buildMonitoringTargetURL("/crawl", opts)
	if err != nil {
		return nil, err
	}
	return c.doMonitoringRequest(requestURL)
}

// ── Cloud Browser API (session-based, distinct shape) ────────────────

func (c *Client) GetBrowserMonitoringMetrics(opts CloudBrowserMonitoringOptions) (map[string]any, error) {
	requestURL, err := c.buildBrowserMonitoringURL("/browser/monitoring/metrics", opts)
	if err != nil {
		return nil, err
	}
	return c.doMonitoringRequest(requestURL)
}

func (c *Client) GetBrowserMonitoringTimeseries(opts CloudBrowserMonitoringOptions) (map[string]any, error) {
	requestURL, err := c.buildBrowserMonitoringURL("/browser/monitoring/metrics/timeseries", opts)
	if err != nil {
		return nil, err
	}
	return c.doMonitoringRequest(requestURL)
}

func (c *Client) buildBrowserMonitoringURL(path string, opts CloudBrowserMonitoringOptions) (string, error) {
	if (!opts.Start.IsZero()) != (!opts.End.IsZero()) {
		return "", fmt.Errorf("cloud browser monitoring: start and end must be provided together")
	}
	endpointURL, _ := url.Parse(c.host + path)
	params := url.Values{}
	params.Set("key", c.key)
	if !opts.Start.IsZero() && !opts.End.IsZero() {
		params.Set("start", opts.Start.UTC().Format(monitoringDatetimeFormat))
		params.Set("end", opts.End.UTC().Format(monitoringDatetimeFormat))
	} else if opts.Period != "" {
		params.Set("period", string(opts.Period))
	}
	if opts.ProxyPool != "" {
		params.Set("proxy_pool", opts.ProxyPool)
	}
	endpointURL.RawQuery = params.Encode()
	return endpointURL.String(), nil
}

func (c *Client) doMonitoringRequest(requestURL string) (map[string]any, error) {
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read monitoring response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleAPIErrorResponse(resp, bodyBytes)
	}

	var data map[string]any
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal monitoring data: %w", err)
	}
	return data, nil
}
