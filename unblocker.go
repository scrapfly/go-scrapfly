package scrapfly

// resolveUnblocker collapses the legacy ASP field and the current Unblocker
// field into the single boolean that goes on the wire.
//
// The wire key stays "asp" on every request. Only the SDK-facing name changed;
// a published module version is immutable and upgraded per installation, so a
// build that emitted "unblocker" against an API deployment that has not learned
// that name yet would silently drop a paid feature — the scrape still succeeds
// and is still billed, but comes back blocked. Renaming the emitted key is a
// separate, later release.
//
// # Precedence
//
// The policy shared by the Python, TypeScript, Go and Rust SDKs is: an
// explicitly supplied legacy value always wins, Unblocker is consulted only
// when the legacy value was not supplied, and the two names are never OR-ed —
// an explicit false on either name must be able to turn the feature off.
//
// Go cannot express that policy exactly. ASP is a bool, so it has no third
// state: "ASP was never set" and "ASP was set to false" are the same value and
// no amount of reflection can tell them apart. The rule implemented here is
// the closest faithful reading:
//
//	ASP == true                  -> true   legacy field forces the feature on
//	ASP == false, Unblocker != nil -> *Unblocker   new field decides, false turns it off
//	ASP == false, Unblocker == nil -> false
//
// That reproduces the shared truth table in every case the type system can
// distinguish. The single divergence is ASP:false together with
// Unblocker:BoolPtr(true): the other SDKs let the explicit false win, here the
// false is indistinguishable from the zero value so Unblocker wins. Callers who
// want the feature off should leave Unblocker nil or pass BoolPtr(false) rather
// than write that contradictory pair.
//
// Unblocker is a *bool and a second field rather than a retype of ASP because
// both alternatives are breaking changes in Go: renaming an exported struct
// field or changing its type stops every existing caller from compiling.
func resolveUnblocker(asp bool, unblocker *bool) bool {
	if asp {
		return true
	}
	if unblocker != nil {
		return *unblocker
	}
	return false
}

// UnblockerEnabled reports whether the anti-bot bypass will be requested — the
// single resolved value that reaches the wire as "asp".
//
// It exists because ASP and Unblocker are two independent exported fields
// rather than one slot behind two names. Without it there is no way to read
// back what a config will actually do: `cfg.Unblocker` is nil after
// `cfg.ASP = true`, and `cfg.ASP` is false after `cfg.Unblocker = BoolPtr(true)`.
// The Python, TypeScript and Rust SDKs all expose the resolved value under both
// names; this is Go's equivalent read surface (Rust spells it
// `unblocker_enabled()`).
func (c *ScrapeConfig) UnblockerEnabled() bool {
	return resolveUnblocker(c.ASP, c.Unblocker)
}

// UnblockerEnabled reports whether the anti-bot bypass will be requested for
// this crawl. See ScrapeConfig.UnblockerEnabled.
func (c *CrawlerConfig) UnblockerEnabled() bool {
	return resolveUnblocker(c.ASP, c.Unblocker)
}
