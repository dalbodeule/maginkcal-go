package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"epdcal/internal/config"
)

func TestEventsCacheDoesNotReuseDifferentRange(t *testing.T) {
	s := NewServer(config.DefaultConfig(), true)
	s.eventsCache = &eventsCache{
		key:       "7\x001",
		resp:      eventsResponse{RangeStart: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)},
		updatedAt: time.Now(),
	}

	request := func(path string) eventsResponse {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body.String())
		}
		var resp eventsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if got := request("/api/events?days=7&backfill=1").RangeStart.Year(); got != 2000 {
		t.Fatalf("matching cache was not used: year = %d", got)
	}
	if got := request("/api/events?days=3&backfill=1").RangeStart.Year(); got == 2000 {
		t.Fatal("different range returned cached response")
	}
}

func TestShouldHighlightRed(t *testing.T) {
	if !shouldHighlightRed("Project deadline", "", []string{"DEADLINE"}) {
		t.Fatal("case-insensitive keyword was not highlighted")
	}
	if shouldHighlightRed("Normal event", "", []string{" ", "urgent"}) {
		t.Fatal("unmatched or blank keyword highlighted the event")
	}
}
