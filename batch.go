package scrapfly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

// BatchResult wraps a single scrape's outcome within a batch response.
// Exactly one of Result, ProxifiedResponse, or Err is non-nil.
// CorrelationID matches the value the caller set on the originating
// ScrapeConfig.
//
// ProxifiedResponse is populated when the originating ScrapeConfig had
// ProxyResponse=true — the batch part's body is the raw upstream
// response (HTML, JSON, binary, etc.) and Upstream response headers
// / status are available on the *http.Response. This mirrors the
// single-scrape Scrape() → ScrapeProxified() split.
type BatchResult struct {
	CorrelationID     string
	Result            *ScrapeResult
	ProxifiedResponse *http.Response
	Err               error
}

// BatchFormat selects the per-part body encoding on the wire.
// JSON is the default; Msgpack produces slightly smaller payloads
// and matches the Scrapfly API's msgpack negotiation used by the
// Python SDK.
type BatchFormat int

const (
	BatchFormatJSON BatchFormat = iota
	BatchFormatMsgpack
)

// BatchOptions carries optional knobs for ScrapeBatchWithOptions.
// Zero value = JSON parts (default), same as the simple ScrapeBatch.
type BatchOptions struct {
	Format BatchFormat
}

// ScrapeBatch issues a POST /scrape/batch request with up to 100
// configs and streams per-scrape results through the returned
// channel as each scrape completes. Results arrive OUT OF ORDER —
// match them back to their originating config via CorrelationID.
//
// Every ScrapeConfig MUST carry a unique CorrelationID. Missing /
// duplicate values are caught client-side before the request is
// sent, so callers fail fast on misconfiguration.
//
// The returned channel is closed when the last part has been
// delivered (or the stream ends due to error). Block on receive.
//
// For msgpack wire encoding use ScrapeBatchWithOptions.
func (c *Client) ScrapeBatch(configs []*ScrapeConfig) (<-chan BatchResult, error) {
	return c.ScrapeBatchWithOptions(configs, BatchOptions{})
}

// ScrapeBatchWithOptions is ScrapeBatch with explicit BatchOptions
// (e.g. msgpack per-part encoding).
func (c *Client) ScrapeBatchWithOptions(configs []*ScrapeConfig, opts BatchOptions) (<-chan BatchResult, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("ScrapeBatch: configs is empty")
	}

	if len(configs) > 100 {
		return nil, fmt.Errorf("ScrapeBatch: max 100 configs per batch (got %d)", len(configs))
	}

	// Client-side correlation_id validation — fail fast.
	seen := make(map[string]int, len(configs))
	configByCorrelation := make(map[string]*ScrapeConfig, len(configs))
	bodyConfigs := make([]map[string]string, 0, len(configs))

	for i, cfg := range configs {
		if cfg.CorrelationID == "" {
			return nil, fmt.Errorf("ScrapeBatch: configs[%d] is missing CorrelationID (required for matching streamed parts)", i)
		}

		if prev, ok := seen[cfg.CorrelationID]; ok {
			return nil, fmt.Errorf("ScrapeBatch: correlation_id %q reused by configs[%d] and configs[%d]", cfg.CorrelationID, prev, i)
		}

		seen[cfg.CorrelationID] = i
		configByCorrelation[cfg.CorrelationID] = cfg

		// Reuse toAPIParamsWithValidation to guarantee wire parity
		// with /scrape. Drop `key` (batch key is in the URL).
		if err := cfg.processBody(); err != nil {
			return nil, fmt.Errorf("ScrapeBatch: configs[%d]: %w", i, err)
		}

		params, err := cfg.toAPIParamsWithValidation()
		if err != nil {
			return nil, fmt.Errorf("ScrapeBatch: configs[%d]: %w", i, err)
		}

		entry := make(map[string]string, len(params))

		for k, v := range params {
			if k == "key" {
				continue
			}

			if len(v) > 0 {
				entry[k] = v[0]
			}
		}

		bodyConfigs = append(bodyConfigs, entry)
	}

	payload, err := json.Marshal(map[string]any{"configs": bodyConfigs})
	if err != nil {
		return nil, fmt.Errorf("ScrapeBatch: marshal body: %w", err)
	}

	endpoint, _ := url.Parse(c.host + "/scrape/batch")
	endpoint.RawQuery = "key=" + url.QueryEscape(c.key)

	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	acceptHeader := "application/json"
	if opts.Format == BatchFormatMsgpack {
		acceptHeader = "application/msgpack"
	}
	req.Header.Set("Accept", acceptHeader)
	// DO NOT set Accept-Encoding explicitly: Go's http.Client
	// auto-decompresses gzip responses only when it added the
	// Accept-Encoding header itself. Setting it manually opts out
	// of auto-decompress and leaves the body as raw gzipped bytes,
	// which would break the multipart parser downstream.
	req.Header.Set("User-Agent", sdkUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ScrapeBatch: http do: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		return nil, c.handleAPIErrorResponse(resp, bodyBytes)
	}

	ct := resp.Header.Get("Content-Type")

	mt, params, parseErr := mime.ParseMediaType(ct)
	if parseErr != nil || mt != "multipart/mixed" {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("ScrapeBatch: expected multipart/mixed, got %q", ct)
	}

	boundary := params["boundary"]
	if boundary == "" {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("ScrapeBatch: Content-Type multipart/mixed is missing boundary: %q", ct)
	}

	// Unbuffered channel: force hand-off to the receiver per part so
	// callers observe streaming order rather than late burst delivery.
	results := make(chan BatchResult)

	go func() {
		defer close(results)
		defer resp.Body.Close()

		mr := multipart.NewReader(resp.Body, boundary)

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				return
			}

			if err != nil {
				results <- BatchResult{
					Err: fmt.Errorf("ScrapeBatch: read part: %w", err),
				}

				return
			}

			correlationID := part.Header.Get("X-Scrapfly-Correlation-Id")

			// Prefer Content-Length over io.ReadAll — Go's
			// multipart.Part.Read scans for the next boundary before
			// returning EOF, which means io.ReadAll(part) for part N
			// will block until part N+1's boundary appears on the
			// wire. Reading exactly Content-Length bytes lets us emit
			// part N the moment its body bytes arrive, preserving
			// per-part streaming end-to-end.
			var partBytes []byte
			var readErr error
			if cl := part.Header.Get("Content-Length"); cl != "" {
				if n, parseErr := strconv.Atoi(cl); parseErr == nil && n >= 0 {
					partBytes = make([]byte, n)
					if _, readErr = io.ReadFull(part, partBytes); readErr != nil {
						partBytes = nil
					}
				}
			}
			if partBytes == nil && readErr == nil {
				partBytes, readErr = io.ReadAll(part)
			}
			_ = part.Close()

			if readErr != nil {
				results <- BatchResult{
					CorrelationID: correlationID,
					Err:           fmt.Errorf("ScrapeBatch: read part body for %q: %w", correlationID, readErr),
				}

				continue
			}

			// Proxified-response parts: the part body is the raw
			// upstream bytes, not a JSON envelope. Surface a native
			// *http.Response synthesized from the part headers + body
			// so callers get the same shape as a single proxified
			// scrape.
			if strings.EqualFold(part.Header.Get("X-Scrapfly-Proxified"), "true") {
				proxResp, buildErr := buildProxifiedResponseFromBatchPart(part.Header, partBytes)
				if buildErr != nil {
					results <- BatchResult{
						CorrelationID: correlationID,
						Err:           fmt.Errorf("ScrapeBatch: build proxified response for %q: %w", correlationID, buildErr),
					}

					continue
				}

				results <- BatchResult{
					CorrelationID:     correlationID,
					ProxifiedResponse: proxResp,
				}

				continue
			}

			partContentType := strings.ToLower(part.Header.Get("Content-Type"))

			var result ScrapeResult
			var decodeErr error

			switch {
			case strings.HasPrefix(partContentType, "application/json"):
				decodeErr = json.Unmarshal(partBytes, &result)
			case strings.HasPrefix(partContentType, "application/msgpack"),
				strings.HasPrefix(partContentType, "application/x-msgpack"):
				// ScrapeResult's fields use `json:` tags; tell the
				// msgpack decoder to honor those so the same struct
				// decodes correctly from either wire format.
				dec := msgpack.NewDecoder(bytes.NewReader(partBytes))
				dec.SetCustomStructTag("json")
				decodeErr = dec.Decode(&result)
			default:
				results <- BatchResult{
					CorrelationID: correlationID,
					Err:           fmt.Errorf("ScrapeBatch: unsupported part Content-Type %q", partContentType),
				}
				continue
			}

			if decodeErr != nil {
				results <- BatchResult{
					CorrelationID: correlationID,
					Err:           fmt.Errorf("ScrapeBatch: unmarshal part for %q: %w", correlationID, decodeErr),
				}

				continue
			}

			results <- BatchResult{
				CorrelationID: correlationID,
				Result:        &result,
			}
		}
	}()

	return results, nil
}

// batchUpstreamPrefix is the header prefix used by the server to
// forward upstream response headers on proxified batch parts
// (avoids collision with the multipart envelope's own headers).
const batchUpstreamPrefix = "X-Scrapfly-Upstream-"

// buildProxifiedResponseFromBatchPart synthesizes an *http.Response
// from a proxified batch part. The part body is the raw upstream
// bytes and the part carries:
//   - Content-Type — the upstream's content-type
//   - X-Scrapfly-Scrape-Status — the upstream's HTTP status
//   - X-Scrapfly-Upstream-<Name> — upstream response headers
//   - X-Scrapfly-Log-Uuid / -Content-Format — scrapfly metadata
//
// Returns an *http.Response with those values restored so the caller
// gets the same shape as a single proxified scrape.
func buildProxifiedResponseFromBatchPart(partHeaders textproto.MIMEHeader, body []byte) (*http.Response, error) {
	status := 200

	if s := partHeaders.Get("X-Scrapfly-Scrape-Status"); s != "" {
		if parsed, err := strconv.Atoi(s); err == nil {
			status = parsed
		}
	}

	outHeaders := http.Header{}

	for key, values := range partHeaders {
		if len(values) == 0 {
			continue
		}

		lower := strings.ToLower(key)

		if lower == "content-type" {
			outHeaders.Set("Content-Type", values[0])
		} else if strings.HasPrefix(key, batchUpstreamPrefix) {
			outHeaders.Set(key[len(batchUpstreamPrefix):], values[0])
		} else if strings.HasPrefix(lower, "x-scrapfly-") {
			outHeaders.Set(key, values[0])
		}
	}

	// Normalize X-Scrapfly-Log-Uuid → X-Scrapfly-Log for parity with
	// the single-scrape proxified response headers.
	if outHeaders.Get("X-Scrapfly-Log") == "" && outHeaders.Get("X-Scrapfly-Log-Uuid") != "" {
		outHeaders.Set("X-Scrapfly-Log", outHeaders.Get("X-Scrapfly-Log-Uuid"))
	}

	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        outHeaders,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}, nil
}
