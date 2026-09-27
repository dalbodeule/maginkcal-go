package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"epdcal/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestICSStatusChecksSavedFeedWithoutLeakingURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.DefaultConfig()
	cfg.ICS = []config.ICSConfig{{ID: "test", URL: "https://example.com/private?token=secret"}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg, true)
	s.SetConfigPath(path)
	s.icsClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.String() != cfg.ICS[0].URL {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		body := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:test-1\r\nDTSTART:20261001T120000Z\r\nDTEND:20261001T130000Z\r\nSUMMARY:쉬는 날\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ics/status", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"state":"ok"`) ||
		!strings.Contains(w.Body.String(), `"event_count":1`) || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
}

func TestICSStatusReportsHTTPFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	got := checkICS(httptest.NewRequest(http.MethodGet, "/api/ics/status", nil), client, config.ICSConfig{ID: "private", URL: "https://example.com/private"})
	if got.State != "http_error" || got.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("status = %+v", got)
	}
}
