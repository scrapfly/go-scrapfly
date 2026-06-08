package scrapfly

import "testing"

// session_sticky_proxy is a *bool: nil = server default (sticky on), &false =
// explicit opt-out, &true = explicit sticky. Sending nothing for an explicit
// false would let the API fall back to its default (sticky=true with a
// session), so the user could never turn proxy stickiness off — the bug
// behind the ipify "same IP" report.

func boolPtr(b bool) *bool { return &b }

func stickyParam(t *testing.T, cfg *ScrapeConfig) (string, bool) {
	t.Helper()
	params, err := cfg.toAPIParamsWithValidation()
	if err != nil {
		t.Fatalf("toAPIParamsWithValidation: %v", err)
	}
	if !params.Has("session_sticky_proxy") {
		return "", false
	}
	return params.Get("session_sticky_proxy"), true
}

func TestSessionStickyProxyFalseIsSent(t *testing.T) {
	cfg := &ScrapeConfig{URL: "https://example.com", Session: "s1", SessionStickyProxy: boolPtr(false)}
	val, present := stickyParam(t, cfg)
	if !present {
		t.Fatal("session_sticky_proxy missing; explicit false must be sent")
	}
	if val != "false" {
		t.Fatalf("session_sticky_proxy = %q, want \"false\"", val)
	}
}

func TestSessionStickyProxyTrueIsSent(t *testing.T) {
	cfg := &ScrapeConfig{URL: "https://example.com", Session: "s1", SessionStickyProxy: boolPtr(true)}
	val, present := stickyParam(t, cfg)
	if !present || val != "true" {
		t.Fatalf("session_sticky_proxy = %q present=%v, want \"true\"", val, present)
	}
}

func TestSessionStickyProxyNilDefersToServerDefault(t *testing.T) {
	// nil = unset: don't send the param, let the API apply its default (true).
	cfg := &ScrapeConfig{URL: "https://example.com", Session: "s1"}
	if _, present := stickyParam(t, cfg); present {
		t.Fatal("session_sticky_proxy must not be sent when unset (nil)")
	}
}

func TestSessionStickyProxyOmittedWithoutSession(t *testing.T) {
	cfg := &ScrapeConfig{URL: "https://example.com", SessionStickyProxy: boolPtr(true)}
	if _, present := stickyParam(t, cfg); present {
		t.Fatal("session_sticky_proxy must not be sent when no session is set")
	}
}
