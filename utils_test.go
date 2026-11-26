package scrapfly

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// holdUntilTheClientLeaves answers nothing until the client goes away.
func holdUntilTheClientLeaves(w http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(10 * time.Second):
	}
}

// closeTheConnection closes the connection without an answer.
func closeTheConnection(w http.ResponseWriter, r *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func answerBadGateway(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadGateway)
}

func TestAScrapeIsSentAgainUnlessItTimedOutAfterTheConnection(t *testing.T) {
	cases := []struct {
		name string
		// answer is the server's handler; nil: every dial times out and no connection is made.
		answer  http.HandlerFunc
		sends   int32
		timeout bool
		// delays is how many retry delays the scrape waited before it returned.
		delays int
	}{
		{"a timeout after the request was sent", holdUntilTheClientLeaves, 1, true, 0},
		{"a dial that times out", nil, 3, true, 2},
		{"a connection closed before an answer", closeTheConnection, 3, false, 2},
		{"a server error", answerBadGateway, 3, false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sends atomic.Int32
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				tc.answer(w, r)
			})
			client.HTTPClient().Timeout = 200 * time.Millisecond
			if tc.answer == nil {
				client.HTTPClient().Transport = &http.Transport{
					DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						sends.Add(1)
						return nil, &net.OpError{Op: "dial", Net: network, Err: os.ErrDeadlineExceeded}
					},
				}
			}

			started := time.Now()

			_, err := client.Scrape(&ScrapeConfig{URL: "https://web-scraping.dev/product/1"})

			if err == nil {
				t.Fatal("the scrape succeeded")
			}
			if got := sends.Load(); got != tc.sends {
				t.Errorf("the scrape was sent %d times, want %d", got, tc.sends)
			}
			if elapsed := time.Since(started); int(elapsed/defaultDelay) != tc.delays {
				t.Errorf("the scrape returned after %s, want %d delays of %s", elapsed, tc.delays, defaultDelay)
			}
			var netErr net.Error
			if timedOut := errors.As(err, &netErr) && netErr.Timeout(); timedOut != tc.timeout {
				t.Errorf("the scrape failed with %v, a timeout: %t, want %t", err, timedOut, tc.timeout)
			}
		})
	}
}
