package scrapfly

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// The SDK-facing name is Unblocker; the wire key stays "asp". These tests pin
// both halves: the precedence between the legacy ASP field and the new
// Unblocker field, and the fact that "unblocker" never reaches the wire.

// unblockerCase is one row of the precedence truth table, shared by the scrape
// param builder and the crawler body builder so the two cannot drift.
type unblockerCase struct {
	name      string
	asp       bool
	unblocker *bool
	// want is the value expected on the wire; wantSet false means the key must
	// be absent entirely (the server then applies its own default).
	want    bool
	wantSet bool
	why     string
}

func unblockerCases() []unblockerCase {
	return []unblockerCase{
		{
			name: "neither", asp: false, unblocker: nil,
			want: false, wantSet: false,
			why: "nothing supplied, key omitted",
		},
		{
			name: "unblocker only true", asp: false, unblocker: BoolPtr(true),
			want: true, wantSet: true,
			why: "new name turns the feature on",
		},
		{
			name: "unblocker only false", asp: false, unblocker: BoolPtr(false),
			want: false, wantSet: false,
			why: "explicit false on the new name must turn the feature off",
		},
		{
			name: "asp only true", asp: true, unblocker: nil,
			want: true, wantSet: true,
			why: "legacy name keeps working forever",
		},
		{
			name: "both true", asp: true, unblocker: BoolPtr(true),
			want: true, wantSet: true,
			why: "agreement, no conflict to resolve",
		},
		{
			name: "conflict asp true unblocker false", asp: true, unblocker: BoolPtr(false),
			want: true, wantSet: true,
			why: "explicitly supplied legacy value wins; ASP=true is the only " +
				"explicit supply a plain bool can express",
		},
		{
			name: "conflict asp false unblocker true", asp: false, unblocker: BoolPtr(true),
			want: true, wantSet: true,
			why: "documented Go divergence: ASP=false is the zero value and " +
				"cannot be told apart from unset, so Unblocker decides",
		},
	}
}

func TestResolveUnblocker_TruthTable(t *testing.T) {
	for _, tc := range unblockerCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUnblocker(tc.asp, tc.unblocker)
			if got != tc.want {
				t.Errorf("resolveUnblocker(%v, %v) = %v, want %v (%s)",
					tc.asp, tc.unblocker, got, tc.want, tc.why)
			}
		})
	}
}

func TestScrapeConfig_UnblockerEmitsAspWireKey(t *testing.T) {
	for _, tc := range unblockerCases() {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &ScrapeConfig{
				URL:       "https://example.com",
				ASP:       tc.asp,
				Unblocker: tc.unblocker,
			}
			params, err := cfg.toAPIParamsWithValidation()
			if err != nil {
				t.Fatalf("toAPIParamsWithValidation: %v", err)
			}

			// The emitted key is "asp", never "unblocker".
			if _, ok := params["unblocker"]; ok {
				t.Fatalf("query must never carry an %q key, got %q", "unblocker", params.Encode())
			}

			_, present := params["asp"]
			if present != tc.wantSet {
				t.Fatalf("asp present = %v, want %v (%s); query=%q",
					present, tc.wantSet, tc.why, params.Encode())
			}
			if tc.wantSet {
				if got := params.Get("asp"); got != "true" {
					t.Errorf("asp = %q, want %q", got, "true")
				}
				if !tc.want {
					t.Fatal("test table is inconsistent: key set but want=false")
				}
			}
		})
	}
}

func TestCrawlerConfig_UnblockerEmitsAspWireKey(t *testing.T) {
	for _, tc := range unblockerCases() {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &CrawlerConfig{
				URL:       "https://example.com",
				ASP:       tc.asp,
				Unblocker: tc.unblocker,
			}
			body, err := cfg.toJSONBody()
			if err != nil {
				t.Fatalf("toJSONBody: %v", err)
			}
			var decoded map[string]interface{}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("unmarshal body: %v", err)
			}

			// The emitted key is "asp", never "unblocker".
			if _, ok := decoded["unblocker"]; ok {
				t.Fatalf("crawl body must never carry an %q key, got %s", "unblocker", body)
			}

			value, present := decoded["asp"]
			if present != tc.wantSet {
				t.Fatalf("asp present = %v, want %v (%s); body=%s",
					present, tc.wantSet, tc.why, body)
			}
			if tc.wantSet && value != true {
				t.Errorf("asp = %v, want true", value)
			}
		})
	}
}

// The multipart path (in-memory URLList) serializes the same config map into
// the "config" part, so the wire key must survive there too.
func TestCrawlerConfig_UnblockerEmitsAspWireKeyMultipart(t *testing.T) {
	cfg := &CrawlerConfig{
		URLList:   []string{"https://example.com/a", "https://example.com/b"},
		Unblocker: BoolPtr(true),
	}
	body, contentType, err := cfg.toMultipartBody()
	if err != nil {
		t.Fatalf("toMultipartBody: %v", err)
	}
	if strings.Contains(string(body), "unblocker") {
		t.Fatalf("multipart body must never carry the %q name, got:\n%s", "unblocker", body)
	}

	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType(%q): %v", contentType, err)
	}
	mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	form, err := mr.ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("ReadForm: %v", err)
	}
	defer form.RemoveAll()

	files := form.File["config"]
	if len(files) != 1 {
		t.Fatalf("expected exactly one config part, got %d", len(files))
	}
	f, err := files[0].Open()
	if err != nil {
		t.Fatalf("open config part: %v", err)
	}
	defer f.Close()

	var decoded map[string]interface{}
	if err := json.NewDecoder(f).Decode(&decoded); err != nil {
		t.Fatalf("decode config part: %v", err)
	}
	if decoded["asp"] != true {
		t.Errorf("config part asp = %v, want true (decoded: %v)", decoded["asp"], decoded)
	}
}

// ErrUnblockerBypassFailed must be the SAME value as ErrASPBypassFailed, not a
// second sentinel — otherwise callers who switched to the new name would stop
// matching errors the SDK wraps with the old one.
func TestErrUnblockerBypassFailed_IsSameSentinel(t *testing.T) {
	if ErrUnblockerBypassFailed != ErrASPBypassFailed {
		t.Fatalf("ErrUnblockerBypassFailed (%v) must be the same value as ErrASPBypassFailed (%v)",
			ErrUnblockerBypassFailed, ErrASPBypassFailed)
	}

	wrappedOld := fmt.Errorf("scrape: %w", ErrASPBypassFailed)
	if !errors.Is(wrappedOld, ErrUnblockerBypassFailed) {
		t.Error("errors.Is(wrap(ErrASPBypassFailed), ErrUnblockerBypassFailed) = false, want true")
	}
	wrappedNew := fmt.Errorf("scrape: %w", ErrUnblockerBypassFailed)
	if !errors.Is(wrappedNew, ErrASPBypassFailed) {
		t.Error("errors.Is(wrap(ErrUnblockerBypassFailed), ErrASPBypassFailed) = false, want true")
	}
}

// The SDK dispatches on the literal "ASP" segment of ERR::ASP::*, which the
// server still sends. Both sentinel names must match what it produces.
func TestCreateErrorFromResult_ASPCodeMatchesBothNames(t *testing.T) {
	client, err := New("test-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result := &ScrapeResult{}
	result.Result.Status = "ERR::ASP::SHIELD_ERROR"
	result.Result.StatusCode = 200
	result.Result.Success = true

	got := client.createErrorFromResult(result)
	if !errors.Is(got, ErrASPBypassFailed) {
		t.Errorf("errors.Is(%v, ErrASPBypassFailed) = false, want true", got)
	}
	if !errors.Is(got, ErrUnblockerBypassFailed) {
		t.Errorf("errors.Is(%v, ErrUnblockerBypassFailed) = false, want true", got)
	}
}

// ---------------------------------------------------------------------------
// Parity matrix: the two names must behave IDENTICALLY, in every case.
// ---------------------------------------------------------------------------
//
// The tests above pin the resolution rule and the wire key. They do not state
// the guarantee a customer migrating from ASP to Unblocker actually leans on:
// the two names produce the SAME request. An assertion targeted at the one key
// cannot prove that, because the names could diverge anywhere else — a second
// key emitted, a key dropped, a different validation outcome. So the tests
// below build two configs that differ ONLY in which name was used and compare
// their WHOLE emitted output: the entire url.Values for the scrape builder and
// the entire body map for the crawler builder.
//
// Go is the one SDK that cannot express the shared rule exactly, because
// ScrapeConfig.ASP/CrawlerConfig.ASP are plain bools with no "unsupplied"
// state. See resolveUnblocker for the documented truth table; every row name in
// unblockerParityRows says whether the row follows the shared rule or is the
// language-forced exception, so the exception cannot be misread later as a bug.

// unblockerParityScrapeBase returns a ScrapeConfig with every unrelated knob
// populated and NEITHER anti-bot field set. Populating the rest is the point of
// a whole-output comparison: a rename that disturbed any other parameter shows
// up as a diff here, where a targeted assertion on "asp" would stay green.
//
// Exactly one cookie: the cookie header is joined by ranging a map, so two
// cookies would order non-deterministically and the comparison would flake for
// a reason unrelated to the rename.
func unblockerParityScrapeBase() *ScrapeConfig {
	return &ScrapeConfig{
		URL:                "https://example.com/parity",
		Country:            "us",
		ProxyPool:          PublicResidentialPool,
		RenderJS:           true,
		WaitForSelector:    "#main",
		RenderingWait:      1500,
		AutoScroll:         true,
		JS:                 "return document.title",
		Screenshots:        map[string]string{"hero": "fullpage"},
		ScreenshotFlags:    []ScreenshotFlag{LoadImages, HighQuality},
		Retry:              true,
		Cache:              true,
		CacheTTL:           120,
		CacheClear:         true,
		Timeout:            30000,
		Debug:              true,
		SSL:                true,
		DNS:                true,
		CorrelationID:      "parity-1",
		Tags:               []string{"a", "b"},
		Webhook:            "hook",
		Session:            "sess-1",
		SessionStickyProxy: BoolPtr(false),
		OS:                 "win11",
		Lang:               []string{"en", "fr"},
		BrowserBrand:       "chrome",
		CostBudget:         500,
		Geolocation:        "48.85,2.35",
		RenderingStage:     "domcontentloaded",
		Format:             FormatMarkdown,
		FormatOptions:      []FormatOption{NoLinks, OnlyContent},
		ExtractionPrompt:   "extract the title",
		Headers:            map[string]string{"X-Custom": "v", "Accept": "text/html"},
		Cookies:            map[string]string{"sid": "42"},
	}
}

// unblockerParityCrawlerBase is the crawler counterpart: every unrelated field
// populated, neither anti-bot field set.
func unblockerParityCrawlerBase() *CrawlerConfig {
	return &CrawlerConfig{
		URL:                       "https://example.com/parity",
		PageLimit:                 10,
		MaxDepth:                  3,
		MaxDuration:               600,
		MaxAPICredit:              5000,
		ExcludePaths:              []string{"/admin/*"},
		IgnoreBasePathRestriction: true,
		FollowExternalLinks:       true,
		AllowedExternalDomains:    []string{"cdn.example.com"},
		FollowInternalSubdomains:  BoolPtr(false),
		AllowedInternalSubdomains: []string{"blog.example.com"},
		Headers:                   map[string]string{"X-Custom": "v", "Accept": "text/html"},
		Delay:                     1000,
		UserAgent:                 "TestBot/1.0",
		MaxConcurrency:            5,
		RenderingDelay:            2000,
		UseSitemaps:               true,
		IgnoreNoFollow:            true,
		RespectRobotsTxt:          BoolPtr(false),
		Cache:                     true,
		CacheTTL:                  3600,
		CacheClear:                true,
		ContentFormats:            []CrawlerContentFormat{CrawlerFormatMarkdown, CrawlerFormatText},
		ExtractionRules:           map[string]interface{}{"title": "h1"},
		Search:                    true,
		Refresh:                   true,
		RefreshInterval:           7200,
		ProxyPool:                 "public_residential_pool",
		Country:                   "us",
		WebhookName:               "my-webhook",
		WebhookEvents:             []CrawlerWebhookEvent{WebhookCrawlerFinished, WebhookCrawlerURLFailed},
	}
}

func unblockerParityScrapeParams(t *testing.T, cfg *ScrapeConfig) url.Values {
	t.Helper()
	params, err := cfg.toAPIParamsWithValidation()
	if err != nil {
		t.Fatalf("toAPIParamsWithValidation: %v", err)
	}
	return params
}

// unblockerParityCrawlerBody returns both the raw POST /crawl body and its
// decoded map. json.Marshal sorts map keys, so the bytes are deterministic and
// worth comparing alongside the decoded map.
func unblockerParityCrawlerBody(t *testing.T, cfg *CrawlerConfig) ([]byte, map[string]interface{}) {
	t.Helper()
	raw, err := cfg.toJSONBody()
	if err != nil {
		t.Fatalf("toJSONBody: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	return raw, decoded
}

// unblockerParityMultipartConfig decodes the "config" part of the multipart
// body used for in-memory URL lists — the second serialization path, which
// must agree with the JSON one.
func unblockerParityMultipartConfig(t *testing.T, cfg *CrawlerConfig) map[string]interface{} {
	t.Helper()
	body, contentType, err := cfg.toMultipartBody()
	if err != nil {
		t.Fatalf("toMultipartBody: %v", err)
	}
	_, mediaParams, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("ParseMediaType(%q): %v", contentType, err)
	}
	form, err := multipart.NewReader(bytes.NewReader(body), mediaParams["boundary"]).ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("ReadForm: %v", err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })

	files := form.File["config"]
	if len(files) != 1 {
		t.Fatalf("expected exactly one config part, got %d", len(files))
	}
	f, err := files[0].Open()
	if err != nil {
		t.Fatalf("open config part: %v", err)
	}
	defer f.Close()

	var decoded map[string]interface{}
	if err := json.NewDecoder(f).Decode(&decoded); err != nil {
		t.Fatalf("decode config part: %v", err)
	}
	return decoded
}

// unblockerEquivalencePair is two ways of writing the same request: the legacy
// name and the current name. The mutators run on a freshly built base config,
// so the two differ in nothing else.
type unblockerEquivalencePair struct {
	name string
	// legacy writes the request the old way, modern the new way.
	legacyScrape, modernScrape   func(*ScrapeConfig)
	legacyCrawler, modernCrawler func(*CrawlerConfig)
	// wantKey is whether "asp" must be on the wire for both members. It also
	// stops the comparison passing vacuously: two builders that emitted nothing
	// would be trivially DeepEqual.
	wantKey bool
	why     string
}

func unblockerEquivalencePairs() []unblockerEquivalencePair {
	return []unblockerEquivalencePair{
		{
			name:          "enabled: ASP true == Unblocker BoolPtr(true)",
			legacyScrape:  func(c *ScrapeConfig) { c.ASP = true },
			modernScrape:  func(c *ScrapeConfig) { c.Unblocker = BoolPtr(true) },
			legacyCrawler: func(c *CrawlerConfig) { c.ASP = true },
			modernCrawler: func(c *CrawlerConfig) { c.Unblocker = BoolPtr(true) },
			wantKey:       true,
			why:           "a customer switching the name on must get byte-for-byte the same request",
		},
		{
			// ASP is a plain bool, so "not supplied" is the only off it can
			// express; that is the state a migrating customer is coming from.
			// The legacy arm writes `ASP = false` explicitly rather than
			// leaving the field alone: in Go the two are the same value by
			// construction, but writing it makes this the named PAIR the other
			// SDKs' disabled rows are (`asp=False` vs `unblocker=False`)
			// instead of "one side never touched the legacy name".
			name:          "disabled: ASP false == Unblocker BoolPtr(false)",
			legacyScrape:  func(c *ScrapeConfig) { c.ASP = false },
			modernScrape:  func(c *ScrapeConfig) { c.Unblocker = BoolPtr(false) },
			legacyCrawler: func(c *CrawlerConfig) { c.ASP = false },
			modernCrawler: func(c *CrawlerConfig) { c.Unblocker = BoolPtr(false) },
			wantKey:       false,
			why:           "switching the name off must not change the request either",
		},
	}
}

// TestUnblockerParity_ScrapeConfig_WholeQueryIdentical compares the ENTIRE
// url.Values of the two names, not just the "asp" key.
func TestUnblockerParity_ScrapeConfig_WholeQueryIdentical(t *testing.T) {
	for _, pair := range unblockerEquivalencePairs() {
		t.Run(pair.name, func(t *testing.T) {
			legacyCfg := unblockerParityScrapeBase()
			pair.legacyScrape(legacyCfg)
			modernCfg := unblockerParityScrapeBase()
			pair.modernScrape(modernCfg)

			legacy := unblockerParityScrapeParams(t, legacyCfg)
			modern := unblockerParityScrapeParams(t, modernCfg)

			// A comparison over a near-empty map would prove nothing; the base
			// config is deliberately rich so a divergence in any other field
			// is observable here.
			if len(legacy) < 25 {
				t.Fatalf("base config emits only %d params; the whole-output comparison "+
					"is only meaningful with the other knobs populated", len(legacy))
			}

			if !reflect.DeepEqual(legacy, modern) {
				t.Fatalf("ASP and Unblocker must emit identical query params (%s)\n legacy: %s\n modern: %s\n  diff: %v",
					pair.why, legacy.Encode(), modern.Encode(), unblockerValuesDiff(legacy, modern))
			}
			// Encode() is the literal wire string; equal maps must encode equally.
			if legacy.Encode() != modern.Encode() {
				t.Fatalf("encoded query differs:\n legacy: %s\n modern: %s", legacy.Encode(), modern.Encode())
			}

			if _, present := legacy["asp"]; present != pair.wantKey {
				t.Fatalf("asp present = %v, want %v — the pair would compare equal vacuously", present, pair.wantKey)
			}
		})
	}
}

// TestUnblockerParity_CrawlerConfig_WholeBodyIdentical compares the ENTIRE
// POST /crawl body of the two names, both decoded and raw.
func TestUnblockerParity_CrawlerConfig_WholeBodyIdentical(t *testing.T) {
	for _, pair := range unblockerEquivalencePairs() {
		t.Run(pair.name, func(t *testing.T) {
			legacyCfg := unblockerParityCrawlerBase()
			pair.legacyCrawler(legacyCfg)
			modernCfg := unblockerParityCrawlerBase()
			pair.modernCrawler(modernCfg)

			legacyRaw, legacy := unblockerParityCrawlerBody(t, legacyCfg)
			modernRaw, modern := unblockerParityCrawlerBody(t, modernCfg)

			if len(legacy) < 25 {
				t.Fatalf("base config emits only %d body keys; the whole-output comparison "+
					"is only meaningful with the other knobs populated", len(legacy))
			}

			if !reflect.DeepEqual(legacy, modern) {
				t.Fatalf("ASP and Unblocker must emit identical crawl bodies (%s)\n legacy: %s\n modern: %s",
					pair.why, legacyRaw, modernRaw)
			}
			// json.Marshal sorts map keys, so equal maps must marshal to equal bytes.
			if !bytes.Equal(legacyRaw, modernRaw) {
				t.Fatalf("raw crawl body differs:\n legacy: %s\n modern: %s", legacyRaw, modernRaw)
			}

			if _, present := legacy["asp"]; present != pair.wantKey {
				t.Fatalf("asp present = %v, want %v — the pair would compare equal vacuously", present, pair.wantKey)
			}
		})
	}
}

// TestUnblockerParity_CrawlerMultipart_WholeConfigPartIdentical covers the
// second crawler serialization path (in-memory URLList), which reuses
// buildBodyMap but through multipart.
func TestUnblockerParity_CrawlerMultipart_WholeConfigPartIdentical(t *testing.T) {
	for _, pair := range unblockerEquivalencePairs() {
		t.Run(pair.name, func(t *testing.T) {
			legacyCfg := unblockerParityCrawlerBase()
			legacyCfg.URL = ""
			legacyCfg.URLList = []string{"https://example.com/a", "https://example.com/b"}
			pair.legacyCrawler(legacyCfg)

			modernCfg := unblockerParityCrawlerBase()
			modernCfg.URL = ""
			modernCfg.URLList = []string{"https://example.com/a", "https://example.com/b"}
			pair.modernCrawler(modernCfg)

			legacy := unblockerParityMultipartConfig(t, legacyCfg)
			modern := unblockerParityMultipartConfig(t, modernCfg)

			if !reflect.DeepEqual(legacy, modern) {
				t.Fatalf("ASP and Unblocker must emit identical multipart config parts (%s)\n legacy: %v\n modern: %v",
					pair.why, legacy, modern)
			}
			if _, present := legacy["asp"]; present != pair.wantKey {
				t.Fatalf("asp present = %v, want %v — the pair would compare equal vacuously", present, pair.wantKey)
			}
		})
	}
}

// TestUnblockerParity_FullTruthTable_CrawlerMultipartWireKey drives every row
// of the truth table — not just the two equivalence pairs — through the SECOND
// crawler serializer. buildBodyMap is shared today, so this is a guard against
// toMultipartBody ever growing a resolution path of its own: a conflict-row
// divergence between the JSON body and the multipart config part would
// otherwise be invisible.
func TestUnblockerParity_FullTruthTable_CrawlerMultipartWireKey(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			cfg := unblockerParityCrawlerBase()
			cfg.URL = ""
			cfg.URLList = []string{"https://example.com/a", "https://example.com/b"}
			cfg.ASP = row.asp
			cfg.Unblocker = row.unblocker

			part := unblockerParityMultipartConfig(t, cfg)

			if _, ok := part["unblocker"]; ok {
				t.Fatalf("the new name must never reach the multipart config part; got %v", part)
			}

			value, present := part["asp"]
			if present != row.wantKey {
				t.Fatalf("asp present = %v, want %v\n  rule: %s\n  part: %v",
					present, row.wantKey, row.rule, part)
			}
			if present && value != true {
				t.Errorf("asp = %v, want true", value)
			}

			// The multipart config part and the JSON body are two renderings of
			// one config; they must agree row for row.
			jsonCfg := unblockerParityCrawlerBase()
			jsonCfg.ASP = row.asp
			jsonCfg.Unblocker = row.unblocker
			_, decoded := unblockerParityCrawlerBody(t, jsonCfg)
			if _, jsonHas := decoded["asp"]; jsonHas != present {
				t.Fatalf("multipart emits asp=%v but the JSON body emits asp=%v for the same input",
					present, jsonHas)
			}
		})
	}
}

// TestUnblockerParity_RowsWithTheSameOutcomeAreIndistinguishable is stronger
// than the per-row assertions: every row that RESOLVES the same way must emit
// the same WHOLE output, whichever name (or pair of names) got the caller
// there. A per-row check on the "asp" key would still pass if two rows agreed
// on the toggle and diverged in some other field. Ported from the Rust matrix
// (unblocker_matrix_rows_with_the_same_outcome_are_indistinguishable) so all
// four SDKs make the claim in its strongest form.
func TestUnblockerParity_RowsWithTheSameOutcomeAreIndistinguishable(t *testing.T) {
	for _, want := range []bool{true, false} {
		t.Run(fmt.Sprintf("resolved=%v", want), func(t *testing.T) {
			var rows []unblockerParityRow
			for _, row := range unblockerParityRows() {
				if row.wantOn == want {
					rows = append(rows, row)
				}
			}
			if len(rows) < 3 {
				t.Fatalf("grouping is only meaningful with several rows per outcome, got %d", len(rows))
			}

			scrapeBase := unblockerParityScrapeBase()
			scrapeBase.ASP = rows[0].asp
			scrapeBase.Unblocker = rows[0].unblocker
			baselineScrape := unblockerParityScrapeParams(t, scrapeBase)
			if len(baselineScrape) < 25 {
				t.Fatalf("base config emits only %d params; the comparison would be near-vacuous", len(baselineScrape))
			}

			crawlerBase := unblockerParityCrawlerBase()
			crawlerBase.ASP = rows[0].asp
			crawlerBase.Unblocker = rows[0].unblocker
			_, baselineCrawler := unblockerParityCrawlerBody(t, crawlerBase)

			for _, row := range rows[1:] {
				scrapeCfg := unblockerParityScrapeBase()
				scrapeCfg.ASP = row.asp
				scrapeCfg.Unblocker = row.unblocker
				got := unblockerParityScrapeParams(t, scrapeCfg)
				if !reflect.DeepEqual(got, baselineScrape) {
					t.Fatalf("row %q diverges from %q although both resolve to %v\n  diff: %v",
						row.name, rows[0].name, want, unblockerValuesDiff(baselineScrape, got))
				}

				crawlerCfg := unblockerParityCrawlerBase()
				crawlerCfg.ASP = row.asp
				crawlerCfg.Unblocker = row.unblocker
				raw, decoded := unblockerParityCrawlerBody(t, crawlerCfg)
				if !reflect.DeepEqual(decoded, baselineCrawler) {
					t.Fatalf("row %q diverges from %q in the crawl body although both resolve to %v\n  body: %s",
						row.name, rows[0].name, want, raw)
				}
			}
		})
	}
}

// unblockerValuesDiff renders the keys that differ between two url.Values, so
// a parity failure names the offending field instead of dumping two blobs.
func unblockerValuesDiff(a, b url.Values) []string {
	var diff []string
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	for k := range seen {
		av, aok := a[k]
		bv, bok := b[k]
		switch {
		case !aok:
			diff = append(diff, fmt.Sprintf("%s: missing in legacy, modern=%v", k, bv))
		case !bok:
			diff = append(diff, fmt.Sprintf("%s: legacy=%v, missing in modern", k, av))
		case !reflect.DeepEqual(av, bv):
			diff = append(diff, fmt.Sprintf("%s: legacy=%v, modern=%v", k, av, bv))
		}
	}
	return diff
}

// unblockerParityRow is one row of the FULL truth table with both names
// present. unblockerCases above covers the resolution rule; this table
// enumerates every combination the two fields can take — including the rows
// Go's type system collapses — and every name states whether the row follows
// the rule shared with the Python / TypeScript / Rust SDKs or is the Go-only,
// language-forced exception.
type unblockerParityRow struct {
	name      string
	asp       bool
	unblocker *bool
	// wantOn is the resolved outcome, wantKey whether "asp" reaches the wire.
	wantOn  bool
	wantKey bool
	rule    string
}

func unblockerParityRows() []unblockerParityRow {
	return []unblockerParityRow{
		{
			name: "neither_supplied/shared_rule",
			asp:  false, unblocker: nil,
			wantOn: false, wantKey: false,
			rule: "nothing supplied: key omitted, server default applies. Same in every SDK.",
		},
		{
			name: "unblocker_only_true/shared_rule",
			asp:  false, unblocker: BoolPtr(true),
			wantOn: true, wantKey: true,
			rule: "ASP not supplied, so Unblocker decides. Same in every SDK.",
		},
		{
			name: "unblocker_only_false/shared_rule",
			asp:  false, unblocker: BoolPtr(false),
			wantOn: false, wantKey: false,
			rule: "explicit false on the new name turns the feature off. Same in every SDK.",
		},
		{
			name: "asp_only_true/shared_rule",
			asp:  true, unblocker: nil,
			wantOn: true, wantKey: true,
			rule: "the legacy name keeps working forever. Same in every SDK.",
		},
		{
			// Not a divergence in outcome: with Unblocker nil the shared rule
			// also resolves to off. It is only the *reason* that differs — the
			// other SDKs honour a supplied false, Go cannot see one. The
			// difference becomes observable only in the conflict row below.
			name: "asp_only_false/go_collapses_supplied_false_into_unset_same_outcome_as_shared_rule",
			asp:  false, unblocker: nil,
			wantOn: false, wantKey: false,
			rule: "ASP=false is the zero value; Go cannot tell it from unset. Outcome " +
				"still matches the shared rule because both say off.",
		},
		{
			name: "both_supplied_agreeing_true/shared_rule",
			asp:  true, unblocker: BoolPtr(true),
			wantOn: true, wantKey: true,
			rule: "agreement, nothing to resolve. Same in every SDK.",
		},
		{
			name: "both_supplied_agreeing_false/shared_rule",
			asp:  false, unblocker: BoolPtr(false),
			wantOn: false, wantKey: false,
			rule: "agreement on off; the names are never OR-ed. Same in every SDK.",
		},
		{
			name: "conflict_asp_true_unblocker_false/shared_rule_supplied_asp_wins",
			asp:  true, unblocker: BoolPtr(false),
			wantOn: true, wantKey: true,
			rule: "an explicitly supplied ASP wins; ASP=true is the only explicit " +
				"supply a plain bool can express. Same in every SDK.",
		},
		{
			// THE one row where Go's answer differs from Python / TypeScript.
			// It is a documented consequence of ASP being a plain bool, not a
			// defect: see resolveUnblocker. Do not "fix" it by making Unblocker
			// lose here — that would break `ScrapeConfig{Unblocker: BoolPtr(true)}`,
			// the ordinary way to turn the feature on.
			name: "conflict_asp_false_unblocker_true/GO_LANGUAGE_FORCED_EXCEPTION_documented_divergence_not_a_bug",
			asp:  false, unblocker: BoolPtr(true),
			wantOn: true, wantKey: true,
			rule: "other SDKs let the explicit ASP=false win and resolve to off. Go " +
				"cannot distinguish that false from the zero value, so Unblocker " +
				"decides and the feature is ON. Documented in resolveUnblocker.",
		},
	}
}

// TestUnblockerParity_FullTruthTable_Resolution pins the resolved outcome of
// every combination of the two names.
func TestUnblockerParity_FullTruthTable_Resolution(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			if got := resolveUnblocker(row.asp, row.unblocker); got != row.wantOn {
				t.Errorf("resolveUnblocker(asp=%v, unblocker=%s) = %v, want %v\n  rule: %s",
					row.asp, unblockerPtrString(row.unblocker), got, row.wantOn, row.rule)
			}
		})
	}
}

// TestUnblockerParity_FullTruthTable_ScrapeWireKey asserts, for every row, the
// emitted key is "asp" with the resolved value and that "unblocker" never
// appears on the wire under either input name.
func TestUnblockerParity_FullTruthTable_ScrapeWireKey(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			cfg := unblockerParityScrapeBase()
			cfg.ASP = row.asp
			cfg.Unblocker = row.unblocker
			params := unblockerParityScrapeParams(t, cfg)

			if _, ok := params["unblocker"]; ok {
				t.Fatalf("the new name must never reach the wire; query=%s", params.Encode())
			}
			if strings.Contains(params.Encode(), "unblocker") {
				t.Fatalf("encoded query must not contain %q anywhere; query=%s", "unblocker", params.Encode())
			}

			value, present := params["asp"]
			if present != row.wantKey {
				t.Fatalf("asp present = %v, want %v\n  rule: %s\n  query: %s",
					present, row.wantKey, row.rule, params.Encode())
			}
			if present {
				if len(value) != 1 || value[0] != "true" {
					t.Errorf("asp = %v, want [true]", value)
				}
				if !row.wantOn {
					t.Fatal("table is inconsistent: key emitted but the resolved outcome is off")
				}
			}
		})
	}
}

// TestUnblockerParity_FullTruthTable_CrawlerWireKey is the crawler counterpart.
func TestUnblockerParity_FullTruthTable_CrawlerWireKey(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			cfg := unblockerParityCrawlerBase()
			cfg.ASP = row.asp
			cfg.Unblocker = row.unblocker
			raw, decoded := unblockerParityCrawlerBody(t, cfg)

			if _, ok := decoded["unblocker"]; ok {
				t.Fatalf("the new name must never reach the wire; body=%s", raw)
			}
			if bytes.Contains(raw, []byte("unblocker")) {
				t.Fatalf("crawl body must not contain %q anywhere; body=%s", "unblocker", raw)
			}

			value, present := decoded["asp"]
			if present != row.wantKey {
				t.Fatalf("asp present = %v, want %v\n  rule: %s\n  body: %s",
					present, row.wantKey, row.rule, raw)
			}
			if present {
				if value != true {
					t.Errorf("asp = %v, want true", value)
				}
				if !row.wantOn {
					t.Fatal("table is inconsistent: key emitted but the resolved outcome is off")
				}
			}
		})
	}
}

// TestUnblockerParity_FullTruthTable_ScrapeAndCrawlerAgree pins the two
// builders against each other: whatever a row resolves to, both serializers
// must make the same call. A divergence here would mean a config behaving one
// way through /scrape and another through /crawl.
func TestUnblockerParity_FullTruthTable_ScrapeAndCrawlerAgree(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			scrapeCfg := unblockerParityScrapeBase()
			scrapeCfg.ASP = row.asp
			scrapeCfg.Unblocker = row.unblocker
			_, scrapeHas := unblockerParityScrapeParams(t, scrapeCfg)["asp"]

			crawlerCfg := unblockerParityCrawlerBase()
			crawlerCfg.ASP = row.asp
			crawlerCfg.Unblocker = row.unblocker
			_, crawlerBody := unblockerParityCrawlerBody(t, crawlerCfg)
			_, crawlerHas := crawlerBody["asp"]

			if scrapeHas != crawlerHas {
				t.Fatalf("scrape emits asp=%v but crawler emits asp=%v for the same input\n  rule: %s",
					scrapeHas, crawlerHas, row.rule)
			}
		})
	}
}

func unblockerPtrString(p *bool) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("BoolPtr(%v)", *p)
}

// ---------------------------------------------------------------------------
// Post-construction parity
// ---------------------------------------------------------------------------
//
// Go exposes the two names as two independent exported struct FIELDS, not as an
// aliased accessor pair, so "set one and read the other back as a field" has no
// meaning here: writing ASP does not write Unblocker. The shared read surface
// is UnblockerEnabled(), the single resolved value that goes on the wire, and
// that is what these tests read back in both directions — the Go equivalent of
// Python's `unblocker` property, TypeScript's accessor and Rust's
// `unblocker_enabled()`.

// TestUnblockerParity_UnblockerEnabledMatchesTheWire pins the read-back half of
// the parity guarantee: for every row of the table, the value a caller can read
// must equal both the row's decided outcome and whether "asp" reaches the wire.
// Without an accessor there is no way to ask a Go config what it will do; that
// is why UnblockerEnabled exists.
func TestUnblockerParity_UnblockerEnabledMatchesTheWire(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			scrapeCfg := unblockerParityScrapeBase()
			scrapeCfg.ASP = row.asp
			scrapeCfg.Unblocker = row.unblocker
			if got := scrapeCfg.UnblockerEnabled(); got != row.wantOn {
				t.Errorf("ScrapeConfig.UnblockerEnabled() = %v, want %v\n  rule: %s", got, row.wantOn, row.rule)
			}
			params := unblockerParityScrapeParams(t, scrapeCfg)
			if _, present := params["asp"]; present != scrapeCfg.UnblockerEnabled() {
				t.Fatalf("UnblockerEnabled() = %v but asp present = %v; the accessor must report the wire",
					scrapeCfg.UnblockerEnabled(), present)
			}

			crawlerCfg := unblockerParityCrawlerBase()
			crawlerCfg.ASP = row.asp
			crawlerCfg.Unblocker = row.unblocker
			if got := crawlerCfg.UnblockerEnabled(); got != row.wantOn {
				t.Errorf("CrawlerConfig.UnblockerEnabled() = %v, want %v\n  rule: %s", got, row.wantOn, row.rule)
			}
			_, decoded := unblockerParityCrawlerBody(t, crawlerCfg)
			if _, present := decoded["asp"]; present != crawlerCfg.UnblockerEnabled() {
				t.Fatalf("UnblockerEnabled() = %v but asp present = %v", crawlerCfg.UnblockerEnabled(), present)
			}
		})
	}
}

// TestUnblockerParity_MixedNameWriteSequence_GO_LANGUAGE_FORCED_EXCEPTION pins
// the SECOND place Go answers differently from the other three SDKs, so nobody
// later reads it as a bug.
//
// Python, TypeScript and Rust keep one storage slot behind two names, so a
// post-construction write through either name overwrites the other and LAST
// WRITE WINS. Go's two names are two independent fields: ASP: true keeps
// winning no matter which was written last, and reading one field back after
// writing the other yields the zero value. Both follow from ASP being a plain
// bool — retyping it to *bool would fix them and would stop every existing
// caller from compiling. Documented on the Unblocker field and in README.md;
// the way to turn the feature off in Go is to clear ASP.
func TestUnblockerParity_MixedNameWriteSequence_GO_LANGUAGE_FORCED_EXCEPTION(t *testing.T) {
	t.Run("last_write_does_not_win", func(t *testing.T) {
		cfg := unblockerParityScrapeBase()
		cfg.ASP = true
		cfg.Unblocker = BoolPtr(false) // in the other SDKs this turns it OFF

		if !cfg.UnblockerEnabled() {
			t.Fatal("expected the feature to stay ON: ASP: true wins over a later Unblocker: BoolPtr(false)")
		}
		params := unblockerParityScrapeParams(t, cfg)
		if _, present := params["asp"]; !present {
			t.Fatalf("asp must still be on the wire; query=%s", params.Encode())
		}

		// Clearing ASP is the documented way off, and it works.
		cfg.ASP = false
		if cfg.UnblockerEnabled() {
			t.Fatal("clearing ASP must let the Unblocker: BoolPtr(false) take effect")
		}
	})

	t.Run("fields_do_not_alias_on_read", func(t *testing.T) {
		byLegacy := unblockerParityScrapeBase()
		byLegacy.ASP = true
		if byLegacy.Unblocker != nil {
			t.Fatalf("Unblocker = %v, want nil: the fields do not alias", byLegacy.Unblocker)
		}
		if !byLegacy.UnblockerEnabled() {
			t.Fatal("UnblockerEnabled() is the read surface that DOES agree")
		}

		byModern := unblockerParityScrapeBase()
		byModern.Unblocker = BoolPtr(true)
		if byModern.ASP {
			t.Fatal("ASP = true, want false: the fields do not alias")
		}
		if !byModern.UnblockerEnabled() {
			t.Fatal("UnblockerEnabled() is the read surface that DOES agree")
		}
	})
}

func TestUnblockerParity_PostConstructionMutation_Scrape(t *testing.T) {
	cfg := unblockerParityScrapeBase()

	assert := func(step string, wantKey bool) {
		t.Helper()
		params := unblockerParityScrapeParams(t, cfg)
		_, present := params["asp"]
		if present != wantKey {
			t.Fatalf("%s: asp present = %v, want %v; query=%s", step, present, wantKey, params.Encode())
		}
	}

	assert("neither field set", false)

	cfg.Unblocker = BoolPtr(true)
	assert("Unblocker set to true after construction", true)

	cfg.Unblocker = BoolPtr(false)
	assert("Unblocker flipped back to false", false)

	cfg.Unblocker = nil
	cfg.ASP = true
	assert("legacy ASP set to true after construction", true)

	cfg.ASP = false
	assert("legacy ASP cleared", false)
}

func TestUnblockerParity_PostConstructionMutation_Crawler(t *testing.T) {
	cfg := unblockerParityCrawlerBase()

	assert := func(step string, wantKey bool) {
		t.Helper()
		raw, decoded := unblockerParityCrawlerBody(t, cfg)
		_, present := decoded["asp"]
		if present != wantKey {
			t.Fatalf("%s: asp present = %v, want %v; body=%s", step, present, wantKey, raw)
		}
	}

	assert("neither field set", false)

	cfg.Unblocker = BoolPtr(true)
	assert("Unblocker set to true after construction", true)

	cfg.Unblocker = BoolPtr(false)
	assert("Unblocker flipped back to false", false)

	cfg.Unblocker = nil
	cfg.ASP = true
	assert("legacy ASP set to true after construction", true)

	cfg.ASP = false
	assert("legacy ASP cleared", false)
}

// TestUnblockerParity_PostConstructionMutation_WholeOutputStillIdentical shows
// the equivalence is a property of the resolution, not of how the struct was
// literal-initialized: mutate one config through the legacy name and the other
// through the new one and the whole outputs still match.
func TestUnblockerParity_PostConstructionMutation_WholeOutputStillIdentical(t *testing.T) {
	t.Run("scrape", func(t *testing.T) {
		legacyCfg := unblockerParityScrapeBase()
		modernCfg := unblockerParityScrapeBase()
		legacyCfg.ASP = true
		modernCfg.Unblocker = BoolPtr(true)

		legacy := unblockerParityScrapeParams(t, legacyCfg)
		modern := unblockerParityScrapeParams(t, modernCfg)
		if !reflect.DeepEqual(legacy, modern) {
			t.Fatalf("post-construction mutation broke parity\n legacy: %s\n modern: %s\n  diff: %v",
				legacy.Encode(), modern.Encode(), unblockerValuesDiff(legacy, modern))
		}
		if _, ok := legacy["asp"]; !ok {
			t.Fatal("asp missing after mutation; the comparison would be vacuous")
		}
	})

	t.Run("crawler", func(t *testing.T) {
		legacyCfg := unblockerParityCrawlerBase()
		modernCfg := unblockerParityCrawlerBase()
		legacyCfg.ASP = true
		modernCfg.Unblocker = BoolPtr(true)

		legacyRaw, legacy := unblockerParityCrawlerBody(t, legacyCfg)
		modernRaw, modern := unblockerParityCrawlerBody(t, modernCfg)
		if !reflect.DeepEqual(legacy, modern) {
			t.Fatalf("post-construction mutation broke parity\n legacy: %s\n modern: %s", legacyRaw, modernRaw)
		}
		if _, ok := legacy["asp"]; !ok {
			t.Fatal("asp missing after mutation; the comparison would be vacuous")
		}
	})
}

// TestUnblockerParity_GoResolutionIsExtensionallyOR records why an "OR the two
// names" mutation cannot be detected in this SDK, so nobody wastes time hunting
// for the test that should have caught it.
//
// The rule shared with the other SDKs is explicitly NOT an OR: an explicit
// false on either name must be able to turn the feature off. In Go it collapses
// into one anyway, because ASP has no supplied-false to honour — over the whole
// input domain, resolveUnblocker is indistinguishable from
// `asp || (unblocker != nil && *unblocker)`. That collapse IS the
// language-forced exception, stated as an equation. It must not be read as
// licence to write the OR in the other SDKs, where the two differ.
func TestUnblockerParity_GoResolutionIsExtensionallyOR(t *testing.T) {
	for _, row := range unblockerParityRows() {
		t.Run(row.name, func(t *testing.T) {
			or := row.asp || (row.unblocker != nil && *row.unblocker)
			if got := resolveUnblocker(row.asp, row.unblocker); got != or {
				t.Fatalf("resolveUnblocker(asp=%v, unblocker=%s) = %v but the OR reading gives %v; "+
					"the Go collapse documented in resolveUnblocker no longer holds",
					row.asp, unblockerPtrString(row.unblocker), got, or)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Client layer
// ---------------------------------------------------------------------------
//
// Every assertion above stops at the config serializer. Nothing pinned that the
// key survives the CLIENT: a param whitelist, a rename shim, or a
// re-serialization between toAPIParamsWithValidation and the outgoing request
// would leave the whole matrix green while the wire lost the flag. These drive
// a real *Client against an httptest server and read the query it actually sent.

// unblockerCapturedScrapeQuery runs one scrape against a local server and
// returns the query string the client put on the wire.
func unblockerCapturedScrapeQuery(t *testing.T, cfg *ScrapeConfig) url.Values {
	t.Helper()

	var captured url.Values
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result": {"status": "DONE", "success": true, "status_code": 200, "content": "", "format": "text"}, "config": {}, "context": {}}`))
	})

	if _, err := client.Scrape(cfg); err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if captured == nil {
		t.Fatal("the client never issued a request")
	}
	return captured
}

func TestUnblockerParity_Client_SendsAspWireKeyUnderEitherName(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*ScrapeConfig)
	}{
		{"legacy ASP", func(c *ScrapeConfig) { c.ASP = true }},
		{"current Unblocker", func(c *ScrapeConfig) { c.Unblocker = BoolPtr(true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := unblockerParityScrapeBase()
			tc.apply(cfg)

			query := unblockerCapturedScrapeQuery(t, cfg)

			if got := query.Get("asp"); got != "true" {
				t.Errorf("asp = %q in the request URL, want %q", got, "true")
			}
			if _, ok := query["unblocker"]; ok {
				t.Errorf("the new name reached the wire: %s", query.Encode())
			}
		})
	}
}

func TestUnblockerParity_Client_OmitsTheKeyWhenOff(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*ScrapeConfig)
	}{
		{"legacy ASP false", func(c *ScrapeConfig) { c.ASP = false }},
		{"current Unblocker false", func(c *ScrapeConfig) { c.Unblocker = BoolPtr(false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := unblockerParityScrapeBase()
			tc.apply(cfg)

			query := unblockerCapturedScrapeQuery(t, cfg)

			if _, ok := query["asp"]; ok {
				t.Errorf("asp emitted while the feature is off: %s", query.Encode())
			}
			if _, ok := query["unblocker"]; ok {
				t.Errorf("the new name reached the wire: %s", query.Encode())
			}
		})
	}
}

// TestUnblockerParity_Client_WholeQueryIdenticalUnderBothNames re-makes the
// whole-output guarantee one layer up: the entire query the client sends, minus
// nothing, must match between the two names.
func TestUnblockerParity_Client_WholeQueryIdenticalUnderBothNames(t *testing.T) {
	legacyCfg := unblockerParityScrapeBase()
	legacyCfg.ASP = true
	modernCfg := unblockerParityScrapeBase()
	modernCfg.Unblocker = BoolPtr(true)

	legacy := unblockerCapturedScrapeQuery(t, legacyCfg)
	modern := unblockerCapturedScrapeQuery(t, modernCfg)

	if len(legacy) < 25 {
		t.Fatalf("client sent only %d params; the whole-output comparison would be near-vacuous", len(legacy))
	}
	if !reflect.DeepEqual(legacy, modern) {
		t.Fatalf("the client must send identical queries for the two names\n legacy: %s\n modern: %s\n  diff: %v",
			legacy.Encode(), modern.Encode(), unblockerValuesDiff(legacy, modern))
	}
	if legacy.Get("asp") != "true" {
		t.Fatal("asp missing from the sent query; the comparison would be vacuous")
	}
}
