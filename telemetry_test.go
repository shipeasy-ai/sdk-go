package shipeasy

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// recordRT records the URL of every request and returns an empty 200 without
// touching the network.
type recordRT struct{ ch chan string }

func (r recordRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.ch <- req.URL.String()
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func recordingClient() (*http.Client, chan string) {
	ch := make(chan string, 16)
	return &http.Client{Transport: recordRT{ch}}, ch
}

func collectURLs(ch chan string, max int, d time.Duration) []string {
	var out []string
	deadline := time.After(d)
	for len(out) < max {
		select {
		case u := <-ch:
			out = append(out, u)
		case <-deadline:
			return out
		}
	}
	return out
}

// 1) basic telemetry send works for each entity call, hitting the right URL.
func TestTelemetryFiresPerEntity(t *testing.T) {
	hc, ch := recordingClient()
	c := NewEngine(Options{APIKey: "k", TelemetryURL: "https://e.x", HTTP: hc})
	c.GetFlag("g", User{})
	c.GetConfig("c")
	c.GetExperiment("e", User{}, nil)

	urls := collectURLs(ch, 3, 2*time.Second)
	if len(urls) != 3 {
		t.Fatalf("want 3 beacons, got %d: %v", len(urls), urls)
	}
	joined := strings.Join(urls, " ")
	for _, suffix := range []string{"/gate/g", "/config/c", "/experiment/e"} {
		if !strings.Contains(joined, suffix) {
			t.Errorf("no beacon ending in %q; got %v", suffix, urls)
		}
	}
	if !strings.Contains(joined, "https://e.x/t/") {
		t.Errorf("beacon did not target the telemetry host: %v", urls)
	}
}

// 2) telemetry is not sent when disabled in settings.
func TestTelemetryDisabled(t *testing.T) {
	hc, ch := recordingClient()
	c := NewEngine(Options{APIKey: "k", TelemetryURL: "https://e.x", HTTP: hc, DisableTelemetry: true})
	c.GetFlag("g", User{})
	c.GetConfig("c")
	c.GetExperiment("e", User{}, nil)

	urls := collectURLs(ch, 1, 300*time.Millisecond)
	if len(urls) != 0 {
		t.Fatalf("want 0 beacons when disabled, got %v", urls)
	}
}
