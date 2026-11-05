package js_scenario

import (
	"encoding/json"
	"testing"
)

func TestSolveCaptchaStep(t *testing.T) {
	steps, err := New().SolveCaptcha(WithCaptchaTimeout(60000), WithCaptchaIgnore()).Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	got, err := json.Marshal(steps)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if want := `[{"solve_captcha":{"timeout":60000,"ignore":true}}]`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestSolveCaptchaStepDefaults(t *testing.T) {
	if _, err := New().SolveCaptcha().Build(); err != nil {
		t.Fatalf("build: %v", err)
	}
}
