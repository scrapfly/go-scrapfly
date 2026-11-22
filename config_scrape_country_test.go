package scrapfly

import (
	"errors"
	"strings"
	"testing"
)

func TestScrapeCountrySyntax(t *testing.T) {
	for _, country := range []string{"", "us", "US", "br,pt,es", "-us,-ca", "us,-ca", "us:1,ca:2", "US:127,-CA", "us:+2,ca:01"} {
		t.Run(country, func(t *testing.T) {
			cfg := &ScrapeConfig{URL: "https://web-scraping.dev", Country: country}
			params, err := cfg.toAPIParamsWithValidation()
			if err != nil {
				t.Fatal(err)
			}
			if got := params.Get("country"); got != strings.ToLower(country) {
				t.Fatalf("country = %q, want %q", got, strings.ToLower(country))
			}
			if country == "" && params.Has("country") {
				t.Fatal("empty country must be omitted")
			}
		})
	}
	for _, country := range []string{",us", "us,", "us,,ca", "-", "-usa", "usa", "us:", "us:0", "us:-1", "us:128", "us:254", "us:256", "us:1:2", "us:1.5", "us:no", "us, ca"} {
		t.Run("invalid_"+country, func(t *testing.T) {
			cfg := &ScrapeConfig{URL: "https://web-scraping.dev", Country: country}
			_, err := cfg.toAPIParamsWithValidation()
			if !errors.Is(err, ErrScrapeConfig) {
				t.Fatalf("expected config error, got %v", err)
			}
		})
	}
}
