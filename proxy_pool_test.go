package scrapfly

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestProxyPoolWireValues(t *testing.T) {
	for pool, want := range map[ProxyPool]string{
		ProxyPoolBasic:          "basic",
		ProxyPoolUnblocker:      "unblocker",
		ProxyPoolUnblockerUltra: "unblocker_ultra",
		ProxyPoolAuto:           "auto",
	} {
		if string(pool) != want {
			t.Errorf("pool constant = %q, want %q", pool, want)
		}
		if !pool.IsValid() {
			t.Errorf("%q.IsValid() = false; the pools are documented values", pool)
		}
		if pool.IsLegacy() {
			t.Errorf("%q.IsLegacy() = true", pool)
		}
	}
}

// The legacy names stay members: dropping one turns its String() into
// "invalid_proxy_pool" and every schema built from AnyEnum rejects it.
func TestProxyPoolEnumKeepsLegacyNames(t *testing.T) {
	for _, legacy := range []ProxyPool{PublicDataCenterPool, PublicResidentialPool} {
		if !legacy.IsValid() {
			t.Errorf("%q.IsValid() = false; legacy names must keep working", legacy)
		}
		if !legacy.IsLegacy() {
			t.Errorf("%q.IsLegacy() = false; it is a legacy pool name", legacy)
		}
	}
	if !PublicTorPool.IsValid() || PublicTorPool.IsLegacy() {
		t.Errorf("%q: IsValid() = %v, IsLegacy() = %v; the Tor pool is a current pool", PublicTorPool, PublicTorPool.IsValid(), PublicTorPool.IsLegacy())
	}
	unknown := ProxyPool("nonexistent_pool")
	if unknown.IsValid() {
		t.Error("an unknown pool name is reported valid")
	}
	if unknown.IsLegacy() {
		t.Error("an unknown pool name is reported legacy")
	}
	if len(GetEnumFor[ProxyPool]()) != len(GetAnyEnumFor[ProxyPool]()) {
		t.Error("Enum() and AnyEnum() list different members")
	}
	want := []any{ProxyPoolBasic, ProxyPoolUnblocker, ProxyPoolUnblockerUltra, ProxyPoolAuto, PublicTorPool, PublicDataCenterPool, PublicResidentialPool}
	if got := GetAnyEnumFor[ProxyPool](); !slices.Equal(got, want) {
		t.Errorf("AnyEnum() = %v, want the current pools first, then the names written before them: %v", got, want)
	}
}

// basic, unblocker, unblocker_ultra and auto go out as proxy_pool with no asp
// key of their own: the API takes the Unblocker from the pool.
func TestScrapeConfigSendsPoolWithoutASP(t *testing.T) {
	for _, pool := range []ProxyPool{ProxyPoolBasic, ProxyPoolUnblocker, ProxyPoolUnblockerUltra, ProxyPoolAuto} {
		params, err := (&ScrapeConfig{URL: "https://web-scraping.dev", ProxyPool: pool}).toAPIParamsWithValidation()
		if err != nil {
			t.Fatalf("toAPIParamsWithValidation: %v", err)
		}
		if got := params.Get("proxy_pool"); got != string(pool) {
			t.Errorf("proxy_pool = %q, want %q", got, pool)
		}
		if _, ok := params["asp"]; ok {
			t.Errorf("pool %q emitted asp=%q", pool, params.Get("asp"))
		}
	}
	params, err := (&ScrapeConfig{URL: "https://web-scraping.dev"}).toAPIParamsWithValidation()
	if err != nil {
		t.Fatalf("toAPIParamsWithValidation: %v", err)
	}
	if _, ok := params["proxy_pool"]; ok {
		t.Errorf("a scrape without a pool emitted proxy_pool=%q; the SDK sends no default", params.Get("proxy_pool"))
	}
}

func TestCrawlerConfigSendsPoolWithoutASP(t *testing.T) {
	for _, pool := range []ProxyPool{ProxyPoolUnblockerUltra, ProxyPoolAuto} {
		body := (&CrawlerConfig{URL: "https://web-scraping.dev", ProxyPool: string(pool)}).buildBodyMap()
		if got := body["proxy_pool"]; got != string(pool) {
			t.Errorf("proxy_pool = %v, want %s", got, pool)
		}
		if _, ok := body["asp"]; ok {
			t.Errorf("pool %s emitted asp=%v", pool, body["asp"])
		}
	}
}

// The answer names the pool the way the account reads it, in the config echo
// and in context.proxy alike: a legacy public_residential_pool order with
// asp=true reads unblocker_ultra. It carries no unblocker_tier, so ConfigData
// declares none.
func TestScrapeResultDecodesCustomerPoolName(t *testing.T) {
	var result ScrapeResult
	if err := json.Unmarshal([]byte(`{
		"config": {"proxy_pool": "unblocker_ultra", "asp": true},
		"context": {"proxy": {"pool": "unblocker_ultra", "network": "residential", "country": "us", "identity": "abc"}}
	}`), &result); err != nil {
		t.Fatal(err)
	}
	if result.Config.ProxyPool != "unblocker_ultra" || !result.Config.ASP {
		t.Errorf("Config.ProxyPool = %q, ASP = %v, want unblocker_ultra and true", result.Config.ProxyPool, result.Config.ASP)
	}
	if proxy := result.Context.Proxy; proxy.Pool != "unblocker_ultra" || proxy.Network != "residential" || proxy.Country != "us" {
		t.Errorf("Context.Proxy = %+v", proxy)
	}

	config := reflect.TypeFor[ConfigData]()
	for i := range config.NumField() {
		if name, _, _ := strings.Cut(config.Field(i).Tag.Get("json"), ","); name == "unblocker_tier" {
			t.Errorf("ConfigData.%s decodes unblocker_tier, a key the API does not answer", config.Field(i).Name)
		}
	}
}

func TestScreenshotConfigSendsProxyPool(t *testing.T) {
	for _, pool := range []ProxyPool{ProxyPoolBasic, ProxyPoolUnblocker, ProxyPoolUnblockerUltra, ProxyPoolAuto} {
		params, err := (&ScreenshotConfig{URL: "https://web-scraping.dev", ProxyPool: pool}).toAPIParams()
		if err != nil {
			t.Fatalf("toAPIParams: %v", err)
		}
		if got := params.Get("proxy_pool"); got != string(pool) {
			t.Errorf("proxy_pool = %q, want %q", got, pool)
		}
	}
	params, err := (&ScreenshotConfig{URL: "https://web-scraping.dev"}).toAPIParams()
	if err != nil {
		t.Fatalf("toAPIParams: %v", err)
	}
	if _, ok := params["proxy_pool"]; ok {
		t.Errorf("a screenshot without a pool emitted proxy_pool=%q", params.Get("proxy_pool"))
	}
}
