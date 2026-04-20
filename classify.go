package scrapfly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ClassifyRequest describes an HTTP response the caller wants the
// Scrapfly classification engine to evaluate for blocking signals.
type ClassifyRequest struct {
	URL        string            `json:"url"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	Method     string            `json:"method,omitempty"`
}

// ClassifyResult is the response from /classify.
type ClassifyResult struct {
	Blocked bool   `json:"blocked"`
	Antibot string `json:"antibot,omitempty"`
	Cost    int    `json:"cost"`
}

// Classify asks the Scrapfly classification endpoint whether the given
// response looks blocked by an anti-bot product. Unlike Scrape, this
// does not fetch anything — the response body is supplied by the caller.
// 1 API credit per call. See
// https://scrapfly.io/docs/scrape-api/classify for the full contract.
func (c *Client) Classify(ctx context.Context, req *ClassifyRequest) (*ClassifyResult, error) {
	if req == nil {
		return nil, errors.New("scrapfly: classify request is nil")
	}
	if req.URL == "" {
		return nil, errors.New("scrapfly: classify url is required")
	}
	if req.StatusCode < 100 || req.StatusCode > 599 {
		return nil, errors.New("scrapfly: classify status_code must be in [100, 599]")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("scrapfly: marshal classify request: %w", err)
	}

	endpointURL, err := url.Parse(c.host + "/classify")
	if err != nil {
		return nil, fmt.Errorf("scrapfly: parse classify url: %w", err)
	}
	params := url.Values{}
	params.Set("key", c.key)
	endpointURL.RawQuery = params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("scrapfly: read classify response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, c.handleAPIErrorResponse(resp, body)
	}

	var out ClassifyResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("scrapfly: decode classify response: %w", err)
	}
	return &out, nil
}
