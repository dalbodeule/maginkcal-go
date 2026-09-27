package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"epdcal/internal/config"
)

func TestManualRefreshBusyAndCooldown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	m := NewRefreshManager(context.Background(), func(context.Context) error {
		close(started)
		<-release
		close(finished)
		return nil
	})
	s := NewServer(&config.Config{}, true)
	s.SetRefreshManager(m)
	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/refresh", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		return w
	}
	if got := post().Code; got != http.StatusAccepted {
		t.Fatalf("first request: got %d", got)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh did not start")
	}
	if got := post().Code; got != http.StatusConflict {
		t.Fatalf("overlapping request: got %d", got)
	}
	if started, _ := m.RunScheduled(context.Background()); started {
		t.Fatal("cron overlapped manual refresh")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("refresh did not finish")
	}
	deadline := time.Now().Add(time.Second)
	for m.Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w := post()
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("cooldown: got %d, retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
}

func TestManualRefreshRequiresJSON(t *testing.T) {
	s := NewServer(&config.Config{}, true)
	s.SetRefreshManager(NewRefreshManager(context.Background(), func(context.Context) error { return nil }))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/refresh", nil))
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("got %d", w.Code)
	}
}

func TestRefreshRejectedAfterShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewRefreshManager(ctx, func(context.Context) error { return nil })
	cancel()
	if _, status := m.start(true); status != http.StatusServiceUnavailable {
		t.Fatalf("got %d", status)
	}
	if started, _ := m.RunScheduled(ctx); started {
		t.Fatal("scheduled refresh started after shutdown")
	}
}
