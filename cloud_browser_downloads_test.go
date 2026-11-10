package scrapfly

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

var (
	smallFile = []byte("%PDF-1.7 small inline file")
	// Not a multiple of 3 bytes, so its base64 ends with padding and the last chunk is short.
	largeFile = func() []byte {
		data := make([]byte, 3*1024*1024+7)
		if _, err := rand.Read(data); err != nil {
			panic(err)
		}
		return data
	}()
	largeBase64 = base64.StdEncoding.EncodeToString(largeFile)
)

type fakeStream struct {
	data string
	off  int
}

// fakeBrowser is the Cloud Browser's side of the download commands, as a CDP session answers them.
type fakeBrowser struct {
	streamed      bool
	orphans       map[string][]byte
	readyAfter    int
	metadataPolls int
	streams       map[string]*fakeStream
	reads         []map[string]any
	closes        []string
}

func newFakeBrowser(streamed bool) *fakeBrowser {
	return &fakeBrowser{streamed: streamed, orphans: map[string][]byte{}, streams: map[string]*fakeStream{}}
}

func (b *fakeBrowser) SendCDP(_ context.Context, method string, params map[string]any) (json.RawMessage, error) {
	result, err := b.answer(method, params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (b *fakeBrowser) answer(method string, params map[string]any) (any, error) {
	switch method {
	case "ScrapiumBrowser.getDownloadsMetadatas":
		b.metadataPolls++
		if b.metadataPolls <= b.readyAfter {
			return map[string]any{"metadata": map[string]any{}}, nil
		}
		return map[string]any{"metadata": map[string]int{"small.pdf": len(smallFile), "large.bin": len(largeFile)}}, nil
	case "ScrapiumBrowser.getDownloads":
		return b.getDownloads(), nil
	case "IO.read":
		return b.read(params)
	case "IO.close":
		handle := params["handle"].(string)
		b.closes = append(b.closes, handle)
		delete(b.streams, handle)
		return map[string]any{}, nil
	}
	return nil, fmt.Errorf("unexpected command %s", method)
}

func (b *fakeBrowser) getDownloads() map[string]any {
	small := base64.StdEncoding.EncodeToString(smallFile)
	if !b.streamed {
		return map[string]any{
			"files":         map[string]string{"small.pdf": small, "large.bin": largeBase64},
			"files_by_guid": map[string]string{"GUID-SMALL": small, "GUID-LARGE": largeBase64},
		}
	}
	b.streams["scrapfly-download:1"] = &fakeStream{data: largeBase64}
	orphans := map[string]string{}
	for name, raw := range b.orphans {
		orphans[name] = base64.StdEncoding.EncodeToString(raw)
	}
	return map[string]any{
		"files":         orphans,
		"files_by_guid": map[string]string{"GUID-SMALL": small},
		"guid_names":    map[string]string{"GUID-SMALL": "small.pdf", "GUID-LARGE": "large.bin"},
		"streams": map[string]any{
			"GUID-LARGE": map[string]any{"stream": "scrapfly-download:1", "size": len(largeFile), "name": "large.bin"},
		},
	}
}

func (b *fakeBrowser) read(params map[string]any) (any, error) {
	b.reads = append(b.reads, params)
	if _, ok := params["offset"]; ok {
		return nil, errors.New("download streams are sequential: IO.read offset is not supported")
	}
	handle := params["handle"].(string)
	stream, ok := b.streams[handle]
	if !ok {
		return nil, fmt.Errorf("Invalid stream handle: %s", handle)
	}
	size := MaxDownloadChunkSize
	if requested, ok := params["size"].(int); ok && requested < size {
		size = requested
	}
	size -= size % 4
	end := min(stream.off+size, len(stream.data))
	chunk := stream.data[stream.off:end]
	stream.off = end
	eof := end >= len(stream.data)
	if eof {
		delete(b.streams, handle)
	}
	return map[string]any{"data": chunk, "base64Encoded": true, "eof": eof}, nil
}

func streamedDownload(t *testing.T, downloads *CloudBrowserDownloads) BrowserDownload {
	t.Helper()
	listed, err := downloads.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, download := range listed {
		if download.Streamed() {
			return download
		}
	}
	t.Fatal("no streamed download listed")
	return BrowserDownload{}
}

func TestCloudBrowserSendsDownloadsStream(t *testing.T) {
	client := &Client{key: "scp-test-key"}
	parsed, err := url.Parse(client.CloudBrowser(&CloudBrowserConfig{DownloadsStream: true}))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("downloads_stream"); got != "true" {
		t.Fatalf("downloads_stream = %q, want true", got)
	}
	if strings.Contains(client.CloudBrowser(&CloudBrowserConfig{}), "downloads_stream") {
		t.Fatal("downloads_stream sent while unset")
	}
}

func TestParseDownloadsStreamedShapeListsSmallFilesInlineAndLargeOnesAsHandles(t *testing.T) {
	browser := newFakeBrowser(true)
	browser.orphans["early.txt"] = []byte("before attach")
	raw, _ := json.Marshal(browser.getDownloads())

	downloads, err := ParseDownloads(raw)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]BrowserDownload{}
	for _, download := range downloads {
		byName[download.Name] = download
	}
	if len(downloads) != 3 || downloads[0].Name != "early.txt" || downloads[2].Name != "small.pdf" {
		t.Fatalf("downloads not listed sorted by name: %+v", downloads)
	}
	if byName["small.pdf"].Streamed() || byName["small.pdf"].Size != int64(len(smallFile)) || byName["small.pdf"].GUID != "GUID-SMALL" {
		t.Fatalf("small.pdf = %+v", byName["small.pdf"])
	}
	if byName["large.bin"].Stream != "scrapfly-download:1" || byName["large.bin"].Size != int64(len(largeFile)) {
		t.Fatalf("large.bin = %+v", byName["large.bin"])
	}
	if byName["early.txt"].GUID != "" || byName["early.txt"].Size != int64(len("before attach")) {
		t.Fatalf("early.txt = %+v", byName["early.txt"])
	}
}

func TestParseDownloadsDefaultShapeListsEachFileOnce(t *testing.T) {
	raw, _ := json.Marshal(newFakeBrowser(false).getDownloads())
	downloads, err := ParseDownloads(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 2 || downloads[0].Name != "large.bin" || downloads[1].Name != "small.pdf" {
		t.Fatalf("downloads = %+v", downloads)
	}
	for _, download := range downloads {
		if download.Streamed() {
			t.Fatalf("%s streamed in the default shape", download.Name)
		}
	}
}

func TestSaveAllWritesEveryFileAndReadsAHandleInChunks(t *testing.T) {
	dir := t.TempDir()
	browser := newFakeBrowser(true)
	browser.orphans["early.txt"] = []byte("before attach")
	downloads := NewCloudBrowserDownloads(browser)
	downloads.ChunkSize = 1024 * 1024

	paths, err := downloads.SaveAll(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "early.txt,large.bin,small.pdf" {
		t.Fatalf("saved %v", names)
	}
	for name, want := range map[string][]byte{"small.pdf": smallFile, "large.bin": largeFile, "early.txt": []byte("before attach")} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %d bytes written, want %d (%v)", name, len(got), len(want), err)
		}
	}
	wantReads := (len(largeBase64) + 1024*1024 - 1) / (1024 * 1024)
	if len(browser.reads) != wantReads {
		t.Fatalf("%d IO.read calls, want %d", len(browser.reads), wantReads)
	}
	for _, read := range browser.reads {
		if read["handle"] != "scrapfly-download:1" || read["size"] != 1024*1024 {
			t.Fatalf("IO.read params %v", read)
		}
	}
	if strings.Join(browser.closes, ",") != "scrapfly-download:1" {
		t.Fatalf("IO.close calls %v", browser.closes)
	}
}

func TestChunkSizeOffTheBase64GridIsRoundedDown(t *testing.T) {
	browser := newFakeBrowser(true)
	downloads := NewCloudBrowserDownloads(browser)
	downloads.ChunkSize = 1000003
	data, err := downloads.Read(context.Background(), streamedDownload(t, downloads))
	if err != nil || !bytes.Equal(data, largeFile) {
		t.Fatalf("read %d bytes (%v)", len(data), err)
	}
	for _, read := range browser.reads {
		if read["size"] != 1000000 {
			t.Fatalf("IO.read size %v, want 1000000", read["size"])
		}
	}
}

func TestChunkSizeOutsideTheServedRangeIsRefused(t *testing.T) {
	for _, size := range []int{2, MaxDownloadChunkSize + 4} {
		browser := newFakeBrowser(true)
		downloads := NewCloudBrowserDownloads(browser)
		downloads.ChunkSize = size
		if _, err := downloads.Read(context.Background(), streamedDownload(t, downloads)); err == nil {
			t.Fatalf("ChunkSize %d accepted", size)
		}
		if len(browser.reads) != 0 {
			t.Fatalf("ChunkSize %d sent IO.read", size)
		}
	}
}

func TestDefaultShapeSavesWithoutAnyIORead(t *testing.T) {
	dir := t.TempDir()
	browser := newFakeBrowser(false)
	if _, err := NewCloudBrowserDownloads(browser).SaveAll(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "large.bin"))
	if !bytes.Equal(got, largeFile) || len(browser.reads) != 0 {
		t.Fatalf("large.bin %d bytes, %d reads", len(got), len(browser.reads))
	}
}

func TestAShortStreamFailsAndStillClosesTheHandle(t *testing.T) {
	browser := newFakeBrowser(true)
	downloads := NewCloudBrowserDownloads(browser)
	large := streamedDownload(t, downloads)
	large.Size++
	if _, err := downloads.Read(context.Background(), large); !errors.Is(err, ErrDownloadIncomplete) {
		t.Fatalf("err = %v, want ErrDownloadIncomplete", err)
	}
	if strings.Join(browser.closes, ",") != "scrapfly-download:1" {
		t.Fatalf("IO.close calls %v", browser.closes)
	}
}

func TestAFailedReadStillClosesTheHandle(t *testing.T) {
	browser := newFakeBrowser(true)
	downloads := NewCloudBrowserDownloads(browser)
	large := streamedDownload(t, downloads)
	large.Stream = "scrapfly-download:404"
	if _, err := downloads.Read(context.Background(), large); err == nil || !strings.Contains(err.Error(), "Invalid stream handle") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(browser.closes, ",") != "scrapfly-download:404" {
		t.Fatalf("IO.close calls %v", browser.closes)
	}
}

func TestACancelledContextStillClosesTheHandle(t *testing.T) {
	browser := newFakeBrowser(true)
	downloads := NewCloudBrowserDownloads(browser)
	large := streamedDownload(t, downloads)
	ctx, cancel := context.WithCancel(context.Background())
	sender := CDPSenderFunc(func(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cancel()
		return browser.SendCDP(ctx, method, params)
	})
	downloads.sender = sender
	downloads.ChunkSize = 1024 * 1024
	if _, err := downloads.Read(ctx, large); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if strings.Join(browser.closes, ",") != "scrapfly-download:1" {
		t.Fatalf("IO.close calls %v", browser.closes)
	}
}

func TestARemoteNameCannotLeaveTheTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	browser := newFakeBrowser(true)
	browser.orphans["../../escape.txt"] = []byte("x")
	paths, err := NewCloudBrowserDownloads(browser).SaveAll(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, path := range paths {
		found = found || path == filepath.Join(dir, "escape.txt")
	}
	if !found {
		t.Fatalf("escape.txt not saved in the directory: %v", paths)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "escape.txt")); err == nil {
		t.Fatal("a remote name wrote outside the directory")
	}
}

func TestSaveToAnExplicitFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "renamed.pdf")
	downloads := NewCloudBrowserDownloads(newFakeBrowser(true))
	listed, _ := downloads.List(context.Background())
	var small BrowserDownload
	for _, download := range listed {
		if download.Name == "small.pdf" {
			small = download
		}
	}
	path, err := downloads.Save(context.Background(), small, target)
	if err != nil || path != target {
		t.Fatalf("Save = %q, %v", path, err)
	}
	if got, _ := os.ReadFile(target); !bytes.Equal(got, smallFile) {
		t.Fatalf("renamed.pdf = %q", got)
	}
}

func TestWaitForReturnsOnceTheSizeSettles(t *testing.T) {
	browser := newFakeBrowser(true)
	browser.readyAfter = 2
	size, err := NewCloudBrowserDownloads(browser).WaitFor(context.Background(), "large.bin", time.Millisecond)
	if err != nil || size != int64(len(largeFile)) || browser.metadataPolls != 4 {
		t.Fatalf("size %d, polls %d, err %v", size, browser.metadataPolls, err)
	}
}

func TestWaitForStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := NewCloudBrowserDownloads(newFakeBrowser(true)).WaitFor(ctx, "missing.bin", 10*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

// fakeChromedpTarget has the method set of chromedp's *chromedp.Target.
type fakeChromedpTarget struct{ browser *fakeBrowser }

func (f fakeChromedpTarget) Execute(ctx context.Context, method string, params, res any) error {
	// chromedp encodes params and decodes the result into res as JSON.
	encoded, err := json.Marshal(params)
	if err != nil {
		return err
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	if size, ok := decoded["size"].(float64); ok {
		decoded["size"] = int(size)
	}
	result, err := f.browser.SendCDP(ctx, method, decoded)
	if err != nil {
		return err
	}
	return json.Unmarshal(result, res)
}

// fakeRodPage has the method set of go-rod's *rod.Page.
type fakeRodPage struct {
	browser  *fakeBrowser
	sessions []string
}

func (f *fakeRodPage) Call(ctx context.Context, sessionID, methodName string, params interface{}) ([]byte, error) {
	f.sessions = append(f.sessions, sessionID)
	return f.browser.SendCDP(ctx, methodName, params.(map[string]any))
}

// fakePlaywrightSession has the method set of playwright-go's CDPSession.
type fakePlaywrightSession struct{ browser *fakeBrowser }

func (f fakePlaywrightSession) Send(method string, params map[string]interface{}) (interface{}, error) {
	raw, err := f.browser.SendCDP(context.Background(), method, params)
	if err != nil {
		return nil, err
	}
	var result interface{}
	err = json.Unmarshal(raw, &result)
	return result, err
}

func TestAdaptersReadAStreamThroughEachLibrary(t *testing.T) {
	rod := &fakeRodPage{}
	cases := map[string]func(*fakeBrowser) CDPSender{
		"chromedp": func(b *fakeBrowser) CDPSender { return ChromedpCDP(fakeChromedpTarget{browser: b}) },
		"rod": func(b *fakeBrowser) CDPSender {
			rod.browser = b
			return RodCDP(rod, "SESSION-1")
		},
		"playwright": func(b *fakeBrowser) CDPSender { return PlaywrightCDP(fakePlaywrightSession{browser: b}) },
	}
	for name, adapter := range cases {
		t.Run(name, func(t *testing.T) {
			browser := newFakeBrowser(true)
			downloads := NewCloudBrowserDownloads(adapter(browser))
			downloads.ChunkSize = 1024 * 1024
			if _, err := downloads.WaitFor(context.Background(), "large.bin", time.Millisecond); err != nil {
				t.Fatal(err)
			}
			data, err := downloads.Read(context.Background(), streamedDownload(t, downloads))
			if err != nil || !bytes.Equal(data, largeFile) {
				t.Fatalf("read %d bytes (%v)", len(data), err)
			}
			if len(browser.reads) < 2 || strings.Join(browser.closes, ",") != "scrapfly-download:1" {
				t.Fatalf("%d reads, closes %v", len(browser.reads), browser.closes)
			}
		})
	}
	for _, session := range rod.sessions {
		if session != "SESSION-1" {
			t.Fatalf("rod called with session %q", session)
		}
	}
}

func TestPlaywrightAdapterStopsOnACancelledContext(t *testing.T) {
	browser := newFakeBrowser(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlaywrightCDP(fakePlaywrightSession{browser: browser}).SendCDP(ctx, "ScrapiumBrowser.getDownloads", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
