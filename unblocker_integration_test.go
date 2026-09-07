//go:build integration

// Integration half of the asp/unblocker parity matrix.
//
// unblocker_test.go already proves the SERIALIZER treats both names alike: it
// compares query strings and multipart bodies byte for byte. That is only half
// the claim. It shows the SDK builds the same request; it cannot show the API
// parses it the same way, because no API is involved. This file closes that
// gap by driving both names through the real client to a real server and
// reading the answer back off the response envelope.
//
//	export SCRAPFLY_API_KEY=scp-live-YOUR_API_KEY_HERE
//	export SCRAPFLY_API_HOST=https://api.scrapfly.io
//	go test -tags=integration -count=1 -timeout=600s -run TestIntegrationUnblockerAliasMatrix -v .
//
// # -count=1 is not optional
//
// Go caches successful test results keyed on the package's inputs, and no
// input to this test changes when the SERVER changes. Without -count=1 a
// re-run prints the previous run's log verbatim — the same uuids, the same
// elapsed time, exit 0 — having made no API call at all. The only tell is
// "(cached)" on the trailing ok line, which most CI summaries drop. For a
// suite whose entire output is "what the live API said", a cached PASS is a
// fabricated one.
//
// # What this file proves, and what it deliberately does not
//
// Be precise, because the obvious reading is wrong. resolveUnblocker collapses
// Unblocker into the single `asp` wire key BEFORE the request is built, so leg
// 1 and leg 2 put a BYTE-IDENTICAL request on the wire. The SDK matrix proves
// the client folds both names onto one key and that the API honours that key.
// It does NOT prove the API still honours the `unblocker` SPELLING, because no
// SDK leg ever sends it.
//
// That spelling is a separate code path in the API itself: it reads the `asp`
// query parameter first and falls back to `unblocker` when `asp` is absent.
//
// It is what a customer on a raw HTTP client depends on, and the API silently
// ignores query params it does not recognise, so deleting it would make
// `unblocker=true` return an UNPROTECTED, billed scrape. Leg 5 is the only leg
// that covers it: it bypasses the SDK's fold and puts `unblocker=true` on the
// wire with no `asp` key. Delete that server-side fallback and leg 5 — and
// only leg 5 — goes red.
//
// # TLS
//
// This file does NOT use integrationClient from crawler_integration_test.go.
// That helper calls NewWithHost(key, host, false), and the third argument is
// verifySSL: false turns into tls.Config{InsecureSkipVerify: true}, which
// means the client cannot tell the real API from anything else holding the
// socket. For a suite whose whole output is "what the API answered", that is
// the wrong footing. aliasIntegrationClient below keeps verification ON and
// trusts the system store, plus any extra root named by SCRAPFLY_CA_BUNDLE.
//
// # Cost, measured rather than assumed
//
// Three legs request the bypass. On this shieldless target that costs the same
// as the disabled legs: context.cost came back {total: 1, details:
// [PROXY_DATACENTER_NETWORK]} for an unblocker=true scrape of httpbin.dev,
// with no anti-bot line item. The ASP surcharge applies when a shield is
// actually engaged, which httpbin.dev does not do. So the fixed leg count is
// discipline about not hammering a live account, not a credit constraint — but
// the no-retry-on-assertion rule still matters for a different reason: a retry
// would hide a genuine alias failure behind a second attempt.
//
// The count is asserted rather than asserted-about: an instrumented
// RoundTripper counts every HTTP attempt that actually leaves the process, so
// Client.Scrape's internal fetchWithRetry (3 attempts on 5xx or on a transport
// error, including a timeout on a slow ASP scrape) cannot silently multiply
// the bill behind a confident "5 legs" summary.
//
// SCRAPFLY_SKIP_BILLABLE=1 runs only the two cheap legs, for debugging the
// harness itself. The test then reports SKIP rather than PASS, so a
// harness-only run can never read as a green alias verdict.
package scrapfly

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// unblockerIntegrationTarget is small, stable and cheap to fetch.
const unblockerIntegrationTarget = "https://httpbin.dev/html"

// legPacing separates the legs.
//
// A project with a throttler configured on the target host reports it back on
// every response under context.throttler. Five legs do not fit inside a
// 5-wide window unless they are spread out, and the window does not clear the
// instant a scrape returns —
// so unpaced back-to-back legs trip ERR::THROTTLE::MAX_CONCURRENT_REQUEST_EXCEEDED
// even though the legs are strictly sequential. Pacing keeps the matrix
// observable; it changes no assertion.
const legPacing = 12 * time.Second

// throttleBackoff is the single wait granted to a throttle-refused leg — long
// enough to drain the host throttler's sliding window rather than land in the
// same one again.
const throttleBackoff = 60 * time.Second

// recordingTransport counts and records every HTTP attempt that actually
// leaves the process.
//
// This is the difference between "the serializer would have produced X" and
// "X went on the wire". Client.Scrape calls toAPIParamsWithValidation
// internally; calling it a second time from the test would only restate the
// unit matrix and would miss anything client.go appends to the URL after the
// params are built. It is also the only honest way to count billable calls:
// fetchWithRetry sits INSIDE Client.Scrape, so counting Scrape invocations
// counts logical legs, not requests.
type recordingTransport struct {
	inner http.RoundTripper
	mu    sync.Mutex
	urls  []string
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.urls = append(rt.urls, req.URL.String())
	rt.mu.Unlock()

	return rt.inner.RoundTrip(req)
}

// since returns the URLs recorded from index n onward.
func (rt *recordingTransport) since(n int) []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	out := make([]string, len(rt.urls[n:]))
	copy(out, rt.urls[n:])

	return out
}

func (rt *recordingTransport) count() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	return len(rt.urls)
}

// aliasIntegrationClient builds a client with TLS verification ON and every
// outbound attempt recorded. It skips (never fails) when either gating
// variable is missing.
//
// Both variables are required. There is deliberately no default host: a
// resolvable default would silently point whatever key is exported at
// whichever endpoint the default names, and a non-resolvable one turns a
// missing variable into a wall of red that reads like an alias regression.
func aliasIntegrationClient(t *testing.T) (*Client, *recordingTransport, string) {
	t.Helper()

	key := os.Getenv("SCRAPFLY_API_KEY")
	if key == "" {
		t.Skip("SCRAPFLY_API_KEY not set — skipping integration test")
	}
	host := os.Getenv("SCRAPFLY_API_HOST")
	if host == "" {
		t.Skip("SCRAPFLY_API_HOST not set — skipping integration test (this file has no default host)")
	}

	client, err := NewWithHost(key, strings.TrimRight(host, "/"), true)
	if err != nil {
		t.Fatal(err)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsNote := "verification ON, system trust store"

	bundle := os.Getenv("SCRAPFLY_CA_BUNDLE")
	if bundle != "" {
		pem, readErr := os.ReadFile(bundle)
		if readErr != nil {
			t.Fatalf("read CA bundle %s: %v", bundle, readErr)
		}
		pool, poolErr := x509.SystemCertPool()
		if poolErr != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			t.Fatalf("CA bundle %s contained no usable certificate", bundle)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool}
		tlsNote = "verification ON, extra root CA trusted from " + bundle
	}

	recorder := &recordingTransport{inner: transport}
	client.SetHTTPClient(&http.Client{Timeout: 150 * time.Second, Transport: recorder})

	return client, recorder, tlsNote
}

// redactKey removes the API key from anything headed for test output. CI logs
// are frequently world-readable and retained for months, and the key rides on
// the query string of every URL here — including the one embedded in a
// *url.Error's own Error() string.
func redactKey(s string) string {
	key := os.Getenv("SCRAPFLY_API_KEY")
	if key == "" {
		return s
	}

	return strings.ReplaceAll(s, key, "scp-live-***REDACTED***")
}

// isThrottleRefusal reports whether the API refused to run the scrape because
// of a concurrency throttle.
//
// A throttle refusal is categorically different from a failed assertion: the
// scrape never executed, nothing was billed, and no outcome was observed. It
// is the API declining to produce a data point, not the API producing a wrong
// one. Only this specific condition is re-dispatched.
func isThrottleRefusal(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.HTTPStatusCode == 429 || strings.HasPrefix(apiErr.Code, "ERR::THROTTLE::")
}

// dispatchLeg runs one leg, giving a throttle refusal exactly one more chance.
//
// This is deliberately NOT a retry on assertion failure: a leg that reaches
// the API and reports the wrong anti-bot state is returned as-is and fails the
// test. Only a refusal that produced no observation at all is re-dispatched,
// and only once, so a persistently throttled project still fails rather than
// spinning. A refused scrape never ran and was never billed, so the extra
// dispatch costs nothing — and the RoundTripper counts it either way.
func dispatchLeg(t *testing.T, client *Client, leg aliasLeg) (*ScrapeResult, error) {
	t.Helper()
	result, err := client.Scrape(leg.config)
	if err == nil || !isThrottleRefusal(err) {
		return result, err
	}
	t.Logf("%s: throttled before the scrape ran (%v) — waiting %s and dispatching once more",
		leg.name, redactKey(err.Error()), throttleBackoff)
	time.Sleep(throttleBackoff)
	return client.Scrape(leg.config)
}

// aliasLeg is one row of the integration matrix.
type aliasLeg struct {
	// name identifies the leg in test output.
	name string
	// config is built fresh per leg; ScrapeConfig carries lazily-populated
	// state, so sharing one across legs would let leg 1 contaminate leg 2.
	config *ScrapeConfig
	// wantEnabled is the anti-bot state the server must report back in
	// config.asp.
	wantEnabled bool
	// billable records whether this leg spends an anti-bot scrape.
	billable bool
}

// aliasObservation is what the API actually told us about a leg.
type aliasObservation struct {
	leg         aliasLeg
	echoedASP   bool
	statusCode  int
	success     bool
	uuid        string
	costTotal   int
	costDetail  string
	contentSize int
	reached     bool
	skipped     bool
	// wireURLs is every HTTP attempt this leg actually put on the wire.
	wireURLs []string
}

// TestIntegrationUnblockerAliasMatrix drives the alias matrix through a live
// API and asserts the two names are indistinguishable at the server.
//
// # Which pair is the honest equivalence comparison
//
// Legs 1 and 2 are, on the CLIENT side: both are affirmative statements by the
// caller — "turn the bypass on" — written under two different names, and the
// SDK must fold them onto the same request. Because it does, they reach the
// server as the same bytes, so at the API they are one observation made twice.
// The server-side claim is leg 5's job, not theirs.
//
// Legs 3 and 4 are NOT an equivalence pair, and calling them one would be
// dishonest — they differ in nothing at all on the wire. In this SDK ASP is a
// plain bool, so ASP:false is the struct's zero value: it is not "the caller
// wrote asp=false", it is "the caller wrote nothing at all". Leg 3 is
// therefore named for what it is, a baseline with no anti-bot field written.
// Unblocker is a *bool and can hold a real third state, so
// Unblocker:BoolPtr(false) IS an explicit negative — but resolveUnblocker
// emits the asp key only when the resolved value is true, so it serializes to
// the same zero bytes as leg 3.
//
// Their job is not to prove the alias symmetric; it is to serve as the control
// that gives legs 1, 2 and 5 their meaning:
//
//   - Leg 3 (default) fixes the baseline: bypass off unless asked for. Without
//     it, an "ENABLED" echo could just be the server's default and the matrix
//     would prove nothing.
//   - Leg 4 proves the new name's explicit false is honoured rather than being
//     read as mere presence of the field — the failure mode where any mention
//     of Unblocker silently bills a bypass. That property is a serializer
//     property, and TestUnblockerParity_ScrapeConfig_WholeQueryIdentical
//     already covers it for free; the round trip here adds only the confirmation
//     that the API agrees with the resulting request.
func TestIntegrationUnblockerAliasMatrix(t *testing.T) {
	client, recorder, tlsNote := aliasIntegrationClient(t)
	host := strings.TrimRight(os.Getenv("SCRAPFLY_API_HOST"), "/")
	skipBillable := os.Getenv("SCRAPFLY_SKIP_BILLABLE") == "1"

	t.Logf("host=%s", host)
	t.Logf("tls=%s", tlsNote)
	if skipBillable {
		t.Logf("SCRAPFLY_SKIP_BILLABLE=1 — the anti-bot legs are NOT running; " +
			"the equivalence claim is NOT being exercised by this run")
	}

	legs := []aliasLeg{
		{
			name:        "1_unblocker_true",
			config:      &ScrapeConfig{URL: unblockerIntegrationTarget, Unblocker: BoolPtr(true)},
			wantEnabled: true,
			billable:    true,
		},
		{
			name:        "2_asp_true",
			config:      &ScrapeConfig{URL: unblockerIntegrationTarget, ASP: true},
			wantEnabled: true,
			billable:    true,
		},
		{
			// Named for what it actually is. ASP is a plain bool, so there is
			// no way to author "legacy name, explicitly disabled" — ASP:false
			// IS the zero value, i.e. no field written at all.
			name:        "3_neither_field_set_baseline",
			config:      &ScrapeConfig{URL: unblockerIntegrationTarget},
			wantEnabled: false,
		},
		{
			name:        "4_unblocker_false",
			config:      &ScrapeConfig{URL: unblockerIntegrationTarget, Unblocker: BoolPtr(false)},
			wantEnabled: false,
		},
	}

	// --- Phase 1: a free fail-fast on the serializer.
	//
	// This is NOT the wire observation — it calls the serializer a second time,
	// independently of the one Client.Scrape performs. It earns its place only
	// by being free and by failing before any credit is spent on the one
	// regression that would make the paid phase meaningless: emitting
	// "unblocker" on the wire. The wire is observed for real in Phase 2, off
	// the recording transport.
	for _, leg := range legs {
		params, err := leg.config.toAPIParamsWithValidation()
		if err != nil {
			t.Fatalf("%s: building query params: %v", leg.name, err)
		}
		if got := params.Get("unblocker"); got != "" {
			t.Fatalf("%s: serializer produced unblocker=%q; the wire key is frozen at \"asp\"", leg.name, got)
		}
		wantASP := ""
		if leg.wantEnabled {
			wantASP = "true"
		}
		if got := params.Get("asp"); got != wantASP {
			t.Fatalf("%s: serializer produced asp=%q, want %q", leg.name, got, wantASP)
		}
	}

	// --- Phase 2: the real calls, once each.
	observations := make([]aliasObservation, 0, len(legs))
	dispatched := 0
	for _, leg := range legs {
		obs := aliasObservation{leg: leg}

		if leg.billable && skipBillable {
			obs.skipped = true
			observations = append(observations, obs)
			t.Logf("%s: SKIPPED (SCRAPFLY_SKIP_BILLABLE=1)", leg.name)
			continue
		}

		if dispatched > 0 {
			time.Sleep(legPacing)
		}
		dispatched++

		before := recorder.count()
		result, err := dispatchLeg(t, client, leg)
		obs.wireURLs = recorder.since(before)
		if err != nil {
			// Cannot compare an outcome we never got. Record the miss and move
			// on rather than retrying — a retry here is another billable call.
			if isThrottleRefusal(err) {
				t.Errorf("%s: THROTTLED before the scrape ran, so this is an environment "+
					"limit and NOT an asp/unblocker divergence: %v", leg.name, redactKey(err.Error()))
			} else {
				t.Errorf("%s: Scrape returned error: %v", leg.name, redactKey(err.Error()))
			}
			observations = append(observations, obs)
			continue
		}
		obs.reached = true
		obs.echoedASP = result.Config.ASP
		obs.statusCode = result.Result.StatusCode
		obs.success = result.Result.Success
		obs.uuid = result.UUID
		obs.costTotal = result.Context.Cost.Total
		obs.costDetail = costCodes(result)
		obs.contentSize = len(result.Result.Content)

		// The request must have genuinely succeeded. Without this, two legs
		// that both blow up identically would satisfy the equivalence check
		// while proving nothing about the alias.
		if obs.statusCode != 200 {
			t.Errorf("%s: upstream status_code=%d, want 200", leg.name, obs.statusCode)
		}
		if !obs.success {
			t.Errorf("%s: result.success=false", leg.name)
		}
		if obs.contentSize == 0 {
			t.Errorf("%s: empty content — the leg did not actually scrape anything", leg.name)
		}

		// The response envelope keeps the old name deliberately: config.asp is
		// frozen regardless of which SDK-side name the caller used.
		if obs.echoedASP != leg.wantEnabled {
			t.Errorf("%s: API echoed config.asp=%v, want %v", leg.name, obs.echoedASP, leg.wantEnabled)
		}

		// The wire, observed rather than re-derived.
		assertWire(t, leg, obs.wireURLs)

		t.Logf("%s: echoed config.asp=%v status_code=%d success=%v cost=%d [%s] content=%dB uuid=%s attempts=%d",
			leg.name, obs.echoedASP, obs.statusCode, obs.success, obs.costTotal,
			obs.costDetail, obs.contentSize, obs.uuid, len(obs.wireURLs))

		observations = append(observations, obs)
	}

	// --- Phase 3: cross-leg equivalence, observed at the API.
	enabledA, enabledB := observations[0], observations[1]
	disabledA, disabledB := observations[2], observations[3]

	if enabledA.skipped || enabledB.skipped {
		t.Log("equivalence NOT exercised: the anti-bot legs were skipped by SCRAPFLY_SKIP_BILLABLE=1")
	} else {
		if !enabledA.reached || !enabledB.reached {
			t.Fatalf("equivalence not testable: one of the enabled legs never reached the API (%s reached=%v, %s reached=%v)",
				enabledA.leg.name, enabledA.reached, enabledB.leg.name, enabledB.reached)
		}

		// The client-side claim: two names, one request, one outcome.
		if enabledA.echoedASP != enabledB.echoedASP {
			t.Errorf("ALIAS BROKEN: %s echoed config.asp=%v but %s echoed config.asp=%v",
				enabledA.leg.name, enabledA.echoedASP, enabledB.leg.name, enabledB.echoedASP)
		}
		if !enabledA.echoedASP || !enabledB.echoedASP {
			t.Errorf("both enabled legs must report the bypass ENABLED; got %s=%v %s=%v",
				enabledA.leg.name, enabledA.echoedASP, enabledB.leg.name, enabledB.echoedASP)
		}
		if enabledA.statusCode != enabledB.statusCode {
			t.Errorf("enabled legs disagree on status_code: %s=%d %s=%d",
				enabledA.leg.name, enabledA.statusCode, enabledB.leg.name, enabledB.statusCode)
		}
	}

	if disabledA.reached && disabledB.reached {
		if disabledA.echoedASP != disabledB.echoedASP {
			t.Errorf("disabled legs disagree: %s echoed config.asp=%v but %s echoed config.asp=%v",
				disabledA.leg.name, disabledA.echoedASP, disabledB.leg.name, disabledB.echoedASP)
		}
		if disabledA.echoedASP || disabledB.echoedASP {
			t.Errorf("both disabled legs must report the bypass DISABLED; got %s=%v %s=%v",
				disabledA.leg.name, disabledA.echoedASP, disabledB.leg.name, disabledB.echoedASP)
		}

		// The control that makes the whole matrix falsifiable: the enabled and
		// disabled halves must actually differ at the API. If the server
		// echoed the same value for all legs, every equivalence assertion
		// above would still pass while observing nothing.
		if enabledA.reached && enabledA.echoedASP == disabledA.echoedASP {
			t.Errorf("config.asp is not observable: enabled and disabled legs both echoed %v — the matrix proves nothing",
				enabledA.echoedASP)
		}
	}

	// --- Phase 4: leg 5, the API-side alias. The only leg that shows the
	// server the new spelling.
	rawEchoed, rawRan := assertAPIHonoursUnblockerAlias(t, client, host, recorder, skipBillable)
	if rawRan && enabledA.reached && rawEchoed != enabledA.echoedASP {
		t.Errorf("a raw `unblocker=true` echoed config.asp=%v but the SDK's `asp=true` echoed %v — "+
			"the two spellings do not reach the same state at the API", rawEchoed, enabledA.echoedASP)
	}

	// --- Phase 5: what the run actually cost, counted at the socket.
	//
	// dispatched legs, not "legs I meant to send": fetchWithRetry lives inside
	// Client.Scrape and re-issues on 5xx AND on transport errors (a timeout on
	// a slow anti-bot scrape included), so a leg can silently become three
	// requests. Counting Scrape calls would report 4 while 12 were billed.
	wantAttempts := dispatched
	if rawRan {
		wantAttempts++
	}
	if got := recorder.count(); got != wantAttempts {
		t.Errorf("the matrix put %d HTTP requests on the wire but dispatched %d legs — "+
			"the SDK retry loop (or a throttle re-dispatch) sent %d extra request(s), "+
			"each one a real scrape on a real account",
			got, wantAttempts, got-wantAttempts)
	}
	t.Logf("matrix complete: %d SDK legs + %d raw alias leg, %d HTTP requests on the wire",
		dispatched, boolToInt(rawRan), recorder.count())

	if skipBillable {
		// Not a PASS. The cheap legs ran and their assertions held, but the
		// equivalence claim and the API-side alias — the two things this file
		// exists to check — were not exercised. Reporting green here would be
		// the exact "passes while doing nothing" failure the suite guards
		// against; SKIP says so in the result line, not only in the log.
		t.Skip("SCRAPFLY_SKIP_BILLABLE=1: harness verified on the cheap legs, but the " +
			"alias equivalence is UNTESTED — unset SCRAPFLY_SKIP_BILLABLE for a verdict")
	}
}

// assertAPIHonoursUnblockerAlias sends `unblocker=true` with no `asp` key and
// asserts the API enabled the bypass anyway.
//
// This is the leg the SDK matrix structurally cannot be. ScrapeConfig resolves
// Unblocker into ASP before the request is built, so no amount of SDK-level
// configuration can produce this request; it is assembled by hand and sent
// through the same instrumented, TLS-verifying client.
func assertAPIHonoursUnblockerAlias(
	t *testing.T, client *Client, host string, recorder *recordingTransport, skipBillable bool,
) (bool, bool) {
	t.Helper()

	if skipBillable {
		t.Log("5_raw_unblocker_alias: SKIPPED (SCRAPFLY_SKIP_BILLABLE=1) — the API-side alias is NOT being exercised")
		return false, false
	}

	time.Sleep(legPacing)

	params := url.Values{}
	params.Set("key", client.APIKey())
	params.Set("url", unblockerIntegrationTarget)
	// `unblocker` alone. An `asp` key of any value wins the server-side
	// precedence rule, and the alias fallback would never be reached.
	params.Set("unblocker", "true")

	endpoint := host + "/scrape?" + params.Encode()

	resp, err := client.HTTPClient().Get(endpoint)
	if err != nil {
		t.Errorf("5_raw_unblocker_alias: request failed: %v", redactKey(err.Error()))
		return false, false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("5_raw_unblocker_alias: reading body: %v", redactKey(err.Error()))
		return false, false
	}

	var envelope struct {
		UUID   string `json:"uuid"`
		Config struct {
			ASP *bool `json:"asp"`
		} `json:"config"`
		Result struct {
			StatusCode int  `json:"status_code"`
			Success    bool `json:"success"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Errorf("5_raw_unblocker_alias: response was not JSON (HTTP %d): %s",
			resp.StatusCode, redactKey(truncate(string(body), 400)))
		return false, false
	}

	// The request has to have worked, or "asp came back false" would be
	// indistinguishable from "the request was rejected".
	if resp.StatusCode != 200 {
		t.Errorf("5_raw_unblocker_alias: API answered HTTP %d: %s",
			resp.StatusCode, redactKey(truncate(string(body), 400)))
		return false, false
	}
	if envelope.Result.StatusCode != 200 || !envelope.Result.Success {
		t.Errorf("5_raw_unblocker_alias: upstream status_code=%d success=%v, want 200/true (uuid=%s)",
			envelope.Result.StatusCode, envelope.Result.Success, envelope.UUID)
		return false, true
	}

	if envelope.Config.ASP == nil {
		t.Errorf("5_raw_unblocker_alias: response envelope carried no config.asp at all (uuid=%s)", envelope.UUID)
		return false, true
	}
	if !*envelope.Config.ASP {
		t.Errorf("THE API-SIDE ALIAS IS BROKEN: a request carrying only `unblocker=true` "+
			"(no `asp` key) was parsed as config.asp=false. Every customer who migrated "+
			"to the new name on a raw HTTP client is being billed for an UNPROTECTED "+
			"scrape. request=%s uuid=%s",
			redactKey(endpoint), envelope.UUID)
		return false, true
	}

	// Prove the request really carried what we think it did, off the recorder
	// rather than off the string we built.
	sent := recorder.since(recorder.count() - 1)
	if len(sent) == 1 {
		if parsed, parseErr := url.Parse(sent[0]); parseErr == nil {
			q := parsed.Query()
			if q.Get("unblocker") != "true" {
				t.Errorf("5_raw_unblocker_alias: harness error, the wire carried unblocker=%q", q.Get("unblocker"))
			}
			if _, present := q["asp"]; present {
				t.Errorf("5_raw_unblocker_alias: harness error, the wire also carried an `asp` key, "+
					"which wins server-side precedence and bypasses the alias fallback: asp=%q", q.Get("asp"))
			}
		}
	}

	t.Logf("5_raw_unblocker_alias: wire carried unblocker=true and NO asp key; "+
		"API echoed config.asp=%v status_code=%d uuid=%s",
		*envelope.Config.ASP, envelope.Result.StatusCode, envelope.UUID)

	return *envelope.Config.ASP, true
}

// assertWire checks the URLs the leg genuinely put on the socket.
func assertWire(t *testing.T, leg aliasLeg, urls []string) {
	t.Helper()

	if len(urls) == 0 {
		t.Errorf("%s: no HTTP request was recorded", leg.name)
		return
	}

	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Errorf("%s: could not parse recorded request url: %v", leg.name, err)
			continue
		}
		q := parsed.Query()

		if got := q.Get("unblocker"); got != "" {
			t.Errorf("%s: `unblocker` reached the wire (=%q); the wire key is frozen at \"asp\": %s",
				leg.name, got, redactKey(raw))
		}
		if leg.wantEnabled {
			if got := q.Get("asp"); got != "true" {
				t.Errorf("%s: expected asp=true on the wire, got %q", leg.name, got)
			}
		} else if _, present := q["asp"]; present {
			// Off is expressed by omission, not by asp=false.
			t.Errorf("%s: expected no `asp` key when the bypass is off, got %q", leg.name, q.Get("asp"))
		}
	}
}

// costCodes renders the cost breakdown so the cost claim in the file header
// stays checkable against what the API actually charged.
func costCodes(result *ScrapeResult) string {
	codes := make([]string, 0, len(result.Context.Cost.Details))
	for _, d := range result.Context.Cost.Details {
		codes = append(codes, d.Code)
	}

	return strings.Join(codes, ",")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
}
