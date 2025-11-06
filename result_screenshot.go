package scrapfly

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ScreenshotResult represents a screenshot captured by the API.
type ScreenshotResult struct {
	// Image contains the raw screenshot image bytes.
	Image []byte
	// Metadata contains information about the screenshot.
	Metadata ScreenshotMetadata
}

// ScreenshotMetadata contains metadata about a captured screenshot.
type ScreenshotMetadata struct {
	// ExtensionName is the file extension (jpg, png, webp, gif).
	ExtensionName string
	// UpstreamStatusCode is the HTTP status code from the target website.
	UpstreamStatusCode int
	// UpstreamURL is the final URL after any redirects.
	UpstreamURL string
}

// newScreenshotResult creates a ScreenshotResult from an HTTP response.
func newScreenshotResult(resp *http.Response, data []byte) (*ScreenshotResult, error) {
	contentType := resp.Header.Get("Content-Type")
	ext := "bin"
	if parts := strings.Split(contentType, "/"); len(parts) == 2 {
		ext = strings.Split(parts[1], ";")[0]
	}

	statusCodeStr := resp.Header.Get("x-scrapfly-upstream-http-code")
	statusCode, _ := strconv.Atoi(statusCodeStr)

	return &ScreenshotResult{
		Image: data,
		Metadata: ScreenshotMetadata{
			ExtensionName:      ext,
			UpstreamStatusCode: statusCode,
			UpstreamURL:        resp.Header.Get("x-scrapfly-upstream-url"),
		},
	}, nil
}

// Save saves a screenshot result to disk.
//
// Parameters:
//   - name: The base name for the file (without extension)
//   - savePath: Optional directory path where to save the file (defaults to current directory)
//
// Returns the full path to the saved file.
//
// Example:
//
//	filePath, err := s.Save("example", "./screenshots")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Screenshot saved to: %s\n", filePath)
func (s *ScreenshotResult) Save(name string, savePath ...string) (string, error) {
	if len(s.Image) == 0 {
		return "", fmt.Errorf("screenshot image is empty")
	}
	dir := "."
	if len(savePath) > 0 {
		dir = savePath[0]
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filePath := filepath.Join(dir, fmt.Sprintf("%s.%s", name, s.Metadata.ExtensionName))
	err := os.WriteFile(filePath, s.Image, 0644)
	return filePath, err
}
