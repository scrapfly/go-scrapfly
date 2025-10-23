package scrapfly

import (
	"encoding/base64"
	"io"
	"net/http"
	"time"
)

// urlSafeB64Encode encodes data into URL-safe base64.
func urlSafeB64Encode(data string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(data))
}

// fetchWithRetry performs an HTTP request with a retry mechanism for 5xx errors.
func fetchWithRetry(client *http.Client, req *http.Request, retries int, delay time.Duration) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt < retries; attempt++ {
		// We need to be able to re-read the body on retries
		var bodyReader io.ReadCloser
		if req.Body != nil {
			var err error
			// GetBody is a function that returns a new reader for the request body
			// This is essential for retries as the body can only be read once.
			bodyReader, err = req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = bodyReader
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			DefaultLogger.Debug("request failed:", err, "retrying...")
			time.Sleep(delay)
			continue
		}

		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			resp.Body.Close() // Close body to prevent resource leaks
			lastErr = &APIError{Message: "server error", HTTPStatusCode: resp.StatusCode}
			DefaultLogger.Debug("request failed with status", resp.StatusCode, "retrying...")
			time.Sleep(delay)
			continue
		}

		return resp, nil
	}
	return nil, lastErr
}
