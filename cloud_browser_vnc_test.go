package scrapfly

import (
	"errors"
	"testing"
)

// The API stores "<project_salt>-<VNCPassword>" and native VNC clients must
// send that exact string, so the separator and the 8-char salt width are a
// wire contract. vncTestSalt is hardcoded rather than recomputed so a change
// to the derivation fails the test instead of moving with it.
const (
	vncTestAPIKey = "scp-test-0000000000000000000000000000000000"
	vncTestSalt   = "701018da"
)

func TestVNCClientPasswordMatchesServerSalting(t *testing.T) {
	cfg := &CloudBrowserConfig{EnableVNC: true, VNCPassword: "hunter2"}

	got, err := cfg.VNCClientPassword(vncTestAPIKey)
	if err != nil {
		t.Fatalf("VNCClientPassword returned error: %v", err)
	}
	if want := vncTestSalt + "-hunter2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestVNCClientPasswordErrorsWhenServerWouldNotSalt(t *testing.T) {
	cases := map[string]*CloudBrowserConfig{
		"nil config":     nil,
		"vnc disabled":   {VNCPassword: "hunter2"},
		"password unset": {EnableVNC: true},
		"password empty": {EnableVNC: true, VNCPassword: ""},
	}

	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := cfg.VNCClientPassword(vncTestAPIKey)
			if !errors.Is(err, ErrVNCNotConfigured) {
				t.Fatalf("got error %v, want ErrVNCNotConfigured", err)
			}
			if got != "" {
				t.Fatalf("got password %q alongside an error, want empty", got)
			}
		})
	}
}
