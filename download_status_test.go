package scrapfly

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// fileAPI serves one file: its first answer is the API's 404 error, the next ones the file.
func fileAPI(t *testing.T, file []byte) *httptest.Server {
	t.Helper()
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message": "not found", "http_code": 404}`))
			return
		}
		_, _ = w.Write(file)
	}))
	t.Cleanup(server.Close)
	return server
}

func assertNotFound(t *testing.T, err error) {
	t.Helper()
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %v, want an *APIError", err)
	}
	if apiErr.HTTPStatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", apiErr.HTTPStatusCode)
	}
}

func TestAnAttachmentAnsweringAnErrorIsNotSavedAndCanBeFetchedAgain(t *testing.T) {
	file := []byte("%PDF-1.4 report")
	server := fileAPI(t, file)
	dir := t.TempDir()
	attachment := &Attachment{Content: server.URL + "/scrape/attachment/01/a1", Filename: "report.pdf"}

	_, err := attachment.Save(dir)

	assertNotFound(t, err)
	if _, statErr := os.Stat(filepath.Join(dir, "report.pdf")); !os.IsNotExist(statErr) {
		t.Fatalf("the error answer was written as the file")
	}

	path, err := attachment.Save(dir)

	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != string(file) {
		t.Fatalf("saved %q, want the file", saved)
	}
}

func TestAScreenshotAnsweringAnErrorIsNotSavedAndCanBeFetchedAgain(t *testing.T) {
	image := []byte("\xff\xd8\xff\xe0 jpeg")
	server := fileAPI(t, image)
	dir := t.TempDir()
	screenshot := &Screenshot{URL: server.URL + "/scrape/screenshot/01/main", Name: "main", Extension: "jpg"}

	_, err := screenshot.Save(dir)

	assertNotFound(t, err)
	if _, statErr := os.Stat(filepath.Join(dir, "main.jpg")); !os.IsNotExist(statErr) {
		t.Fatalf("the error answer was written as the image")
	}

	path, err := screenshot.Save(dir)

	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	saved, _ := os.ReadFile(path)
	if string(saved) != string(image) {
		t.Fatalf("saved %q, want the image", saved)
	}
}
