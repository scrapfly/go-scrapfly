package scrapfly

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Retrieving the files a Cloud Browser session downloaded.
//
// Connect with CloudBrowserConfig{DownloadsStream: true}: ScrapiumBrowser.getDownloads
// then returns each small file inline and each large one as an IO stream handle, which
// CloudBrowserDownloads reads in chunks with IO.read, so no CDP message is larger than
// one chunk. Without the option every file comes back inline, and the helpers read that
// shape too.
//
//	downloads := scrapfly.NewCloudBrowserDownloads(scrapfly.ChromedpCDP(chromedp.FromContext(ctx).Target))
//	if _, err := downloads.WaitFor(ctx, "report.pdf", 0); err != nil { ... }
//	paths, err := downloads.SaveAll(ctx, "downloads")

const (
	// DefaultDownloadChunkSize is the base64 characters read per IO.read: an
	// 8 MiB message carrying 6 MiB of the file.
	DefaultDownloadChunkSize = 8 * 1024 * 1024
	// MaxDownloadChunkSize is the largest chunk the Cloud Browser serves per IO.read.
	MaxDownloadChunkSize = 32 * 1024 * 1024

	defaultDownloadWaitInterval = 500 * time.Millisecond
	downloadCloseTimeout        = 10 * time.Second
)

// ErrDownloadIncomplete reports a stream that ended with fewer or more bytes than
// the download's size.
var ErrDownloadIncomplete = errors.New("download stream ended with the wrong size")

// CDPSender sends one CDP command on a Cloud Browser session and returns its result.
type CDPSender interface {
	SendCDP(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)
}

// CDPSenderFunc adapts a function to CDPSender, for a CDP client of your own.
type CDPSenderFunc func(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)

// SendCDP calls f.
func (f CDPSenderFunc) SendCDP(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	return f(ctx, method, params)
}

// ChromedpExecutor is the method set of chromedp's *chromedp.Target (cdp.Executor,
// chromedp v0.10 and later).
type ChromedpExecutor interface {
	Execute(ctx context.Context, method string, params, res any) error
}

// ChromedpCDP sends through chromedp: pass chromedp.FromContext(ctx).Target.
func ChromedpCDP(executor ChromedpExecutor) CDPSender {
	return CDPSenderFunc(func(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
		var result json.RawMessage
		if err := executor.Execute(ctx, method, params, &result); err != nil {
			return nil, err
		}
		return result, nil
	})
}

// RodCaller is the method set of go-rod's *rod.Page and *rod.Browser.
type RodCaller interface {
	Call(ctx context.Context, sessionID, methodName string, params interface{}) ([]byte, error)
}

// RodCDP sends through go-rod: pass the page and string(page.SessionID).
func RodCDP(caller RodCaller, sessionID string) CDPSender {
	return CDPSenderFunc(func(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
		result, err := caller.Call(ctx, sessionID, method, params)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(result), nil
	})
}

// PlaywrightCDPSession is the method set of playwright-go's CDPSession.
type PlaywrightCDPSession interface {
	Send(method string, params map[string]interface{}) (interface{}, error)
}

// PlaywrightCDP sends through playwright-go: pass context.NewCDPSession(page).
// playwright-go takes no context, so a cancelled ctx only stops between commands.
func PlaywrightCDP(session PlaywrightCDPSession) CDPSender {
	return CDPSenderFunc(func(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := session.Send(method, params)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
}

// BrowserDownload is one file of the session's download directory. A small file
// carries its base64 content in Data. A large one, with DownloadsStream, carries the
// IO handle to read in Stream instead, and its size in bytes in Size.
type BrowserDownload struct {
	Name   string
	GUID   string
	Size   int64
	Data   string
	Stream string
}

// Streamed reports whether the download is read through an IO stream handle.
func (d BrowserDownload) Streamed() bool {
	return d.Stream != ""
}

type downloadStreamEntry struct {
	Stream string `json:"stream"`
	Size   int64  `json:"size"`
	Name   string `json:"name"`
}

type getDownloadsResult struct {
	Files       map[string]string              `json:"files"`
	FilesByGUID map[string]string              `json:"files_by_guid"`
	GUIDNames   map[string]string              `json:"guid_names"`
	Streams     map[string]downloadStreamEntry `json:"streams"`
}

// ParseDownloads reads a ScrapiumBrowser.getDownloads result, in either of its
// shapes, sorted by name.
func ParseDownloads(raw json.RawMessage) ([]BrowserDownload, error) {
	var result getDownloadsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("ScrapiumBrowser.getDownloads: %w", err)
	}

	var downloads []BrowserDownload
	if result.GUIDNames != nil || result.Streams != nil {
		for guid, data := range result.FilesByGUID {
			name := result.GUIDNames[guid]
			if name == "" {
				name = guid
			}
			downloads = append(downloads, BrowserDownload{Name: name, GUID: guid, Size: decodedSize(data), Data: data})
		}
		for guid, entry := range result.Streams {
			name := entry.Name
			if name == "" {
				name = result.GUIDNames[guid]
			}
			if name == "" {
				name = guid
			}
			downloads = append(downloads, BrowserDownload{Name: name, GUID: guid, Size: entry.Size, Stream: entry.Stream})
		}
		// A file on disk with no download GUID, such as one finished before this session attached.
		for name, data := range result.Files {
			downloads = append(downloads, BrowserDownload{Name: name, Size: decodedSize(data), Data: data})
		}
	} else {
		// Without downloads_stream every file is inline under files, and again under
		// files_by_guid with no name: files alone holds each file once.
		for name, data := range result.Files {
			downloads = append(downloads, BrowserDownload{Name: name, Size: decodedSize(data), Data: data})
		}
	}

	sort.Slice(downloads, func(i, j int) bool {
		return downloads[i].Name < downloads[j].Name
	})
	return downloads, nil
}

func decodedSize(data string) int64 {
	padding := 0
	if strings.HasSuffix(data, "==") {
		padding = 2
	} else if strings.HasSuffix(data, "=") {
		padding = 1
	}
	return int64(len(data)/4*3 - padding)
}

// CloudBrowserDownloads reads the downloads of a Cloud Browser session.
type CloudBrowserDownloads struct {
	sender CDPSender
	// ChunkSize is the base64 characters read per IO.read, rounded down to a
	// multiple of 4: DefaultDownloadChunkSize when zero, MaxDownloadChunkSize at most.
	ChunkSize int
}

// NewCloudBrowserDownloads reads downloads through sender: ChromedpCDP, RodCDP,
// PlaywrightCDP, or a CDPSenderFunc of your own.
func NewCloudBrowserDownloads(sender CDPSender) *CloudBrowserDownloads {
	return &CloudBrowserDownloads{sender: sender, ChunkSize: DefaultDownloadChunkSize}
}

func (d *CloudBrowserDownloads) chunkSize() (int, error) {
	size := d.ChunkSize
	if size == 0 {
		size = DefaultDownloadChunkSize
	}
	if size < 4 || size > MaxDownloadChunkSize {
		return 0, fmt.Errorf("ChunkSize must be between 4 and %d base64 characters, got %d", MaxDownloadChunkSize, size)
	}
	return size - size%4, nil
}

// Metadata returns the size in bytes of every completed download, by filename.
func (d *CloudBrowserDownloads) Metadata(ctx context.Context) (map[string]int64, error) {
	raw, err := d.sender.SendCDP(ctx, "ScrapiumBrowser.getDownloadsMetadatas", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Metadata map[string]int64 `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("ScrapiumBrowser.getDownloadsMetadatas: %w", err)
	}
	if result.Metadata == nil {
		result.Metadata = map[string]int64{}
	}
	return result.Metadata, nil
}

// WaitFor waits until filename is listed with the same size twice in a row and
// returns that size, checking every interval (500ms when zero). Bound it with ctx.
func (d *CloudBrowserDownloads) WaitFor(ctx context.Context, filename string, interval time.Duration) (int64, error) {
	if interval <= 0 {
		interval = defaultDownloadWaitInterval
	}
	last := int64(-1)
	for {
		metadata, err := d.Metadata(ctx)
		if err != nil {
			return 0, err
		}
		size, listed := metadata[filename]
		if !listed {
			size = -1
		}
		if size >= 0 && size == last {
			return size, nil
		}
		last = size
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("%s did not finish downloading: %w", filename, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// List returns every download of the session: small ones with their content,
// large ones as handles.
func (d *CloudBrowserDownloads) List(ctx context.Context) ([]BrowserDownload, error) {
	raw, err := d.sender.SendCDP(ctx, "ScrapiumBrowser.getDownloads", map[string]any{})
	if err != nil {
		return nil, err
	}
	return ParseDownloads(raw)
}

// WriteTo writes the bytes of a download to w, one decoded chunk at a time, and
// returns how many it wrote. A stream handle is read once and always closed.
func (d *CloudBrowserDownloads) WriteTo(ctx context.Context, download BrowserDownload, w io.Writer) (int64, error) {
	if !download.Streamed() {
		data, err := base64.StdEncoding.DecodeString(download.Data)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", download.Name, err)
		}
		written, err := w.Write(data)
		return int64(written), err
	}

	size, err := d.chunkSize()
	if err != nil {
		return 0, err
	}
	defer d.closeStream(ctx, download.Stream)

	var written int64
	for {
		raw, err := d.sender.SendCDP(ctx, "IO.read", map[string]any{"handle": download.Stream, "size": size})
		if err != nil {
			return written, err
		}
		var reply struct {
			Data          string `json:"data"`
			Base64Encoded *bool  `json:"base64Encoded"`
			EOF           bool   `json:"eof"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			return written, fmt.Errorf("IO.read: %w", err)
		}
		if reply.Data != "" {
			chunk := []byte(reply.Data)
			if reply.Base64Encoded == nil || *reply.Base64Encoded {
				chunk, err = base64.StdEncoding.DecodeString(reply.Data)
				if err != nil {
					return written, fmt.Errorf("IO.read: %w", err)
				}
			}
			n, err := w.Write(chunk)
			written += int64(n)
			if err != nil {
				return written, err
			}
		}
		if reply.EOF {
			break
		}
	}

	if download.Size > 0 && written != download.Size {
		return written, fmt.Errorf("%s: read %d bytes, expected %d: %w", download.Name, written, download.Size, ErrDownloadIncomplete)
	}
	return written, nil
}

// closeStream releases a handle even when ctx is already cancelled.
func (d *CloudBrowserDownloads) closeStream(ctx context.Context, handle string) {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), downloadCloseTimeout)
	defer cancel()
	_, _ = d.sender.SendCDP(closeCtx, "IO.close", map[string]any{"handle": handle})
}

// Read returns the whole content of a download, in memory.
func (d *CloudBrowserDownloads) Read(ctx context.Context, download BrowserDownload) ([]byte, error) {
	var buffer bytes.Buffer
	if _, err := d.WriteTo(ctx, download, &buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Save writes a download to path, a file or a directory (the current one when
// empty), chunk by chunk, and returns the path written.
func (d *CloudBrowserDownloads) Save(ctx context.Context, download BrowserDownload, path string) (string, error) {
	target := downloadTargetPath(download, path)
	file, err := os.Create(target)
	if err != nil {
		return "", err
	}
	if _, err := d.WriteTo(ctx, download, file); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return target, nil
}

// SaveAll writes every download of the session into dir and returns their paths.
func (d *CloudBrowserDownloads) SaveAll(ctx context.Context, dir string) ([]string, error) {
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	downloads, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, download := range downloads {
		path, err := d.Save(ctx, download, dir)
		if err != nil {
			return paths, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func downloadTargetPath(download BrowserDownload, path string) string {
	// The name comes from the remote browser: keep its last component only.
	name := filepath.Base(strings.ReplaceAll(download.Name, "\\", "/"))
	if name == "." || name == "/" || name == ".." {
		name = download.GUID
	}
	if name == "" {
		name = "download"
	}
	if path == "" {
		return name
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return filepath.Join(path, name)
	}
	return path
}
