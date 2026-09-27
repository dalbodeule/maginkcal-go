package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"epdcal/internal/config"
	"epdcal/internal/ics"
	"epdcal/internal/model"
)

func TestEventsCacheDoesNotReuseDifferentRange(t *testing.T) {
	s := NewServer(config.DefaultConfig(), true)
	s.eventsCache = &eventsCache{
		key:       "7\x001",
		config:    s.Config(),
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
	updated := *s.Config()
	s.current.Store(&updated)
	if got := request("/api/events?days=7&backfill=1").RangeStart.Year(); got == 2000 {
		t.Fatal("old config cache was reused after live update")
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

func TestHolidayPrefixMatchesOnlyTitleStart(t *testing.T) {
	if !isHoliday(" 쉬는 날 어린이날", []string{"쉬는 날"}) {
		t.Fatal("leading whitespace should not hide a holiday")
	}
	if isHoliday("오늘은 쉬는 날", []string{"쉬는 날"}) ||
		isHoliday("쉬는 날", []string{}) ||
		isHoliday("평일", []string{""}) {
		t.Fatal("non-prefix or disabled rule matched")
	}
}

func TestHolidayDatesCoverMultiDayEventWithExclusiveEnd(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 10, 3, 0, 0, 0, 0, loc)
	got := holidayDateKeys(start, end, loc, start, end.AddDate(0, 0, 3))
	if len(got) != 2 || got[0] != "2026-10-01" || got[1] != "2026-10-02" {
		t.Fatalf("holiday dates = %v", got)
	}
}

func TestHolidayDateRemainsWhenAllDayEventsAreHidden(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ShowAllDay = false
	loc := time.UTC
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	result := buildEventsResponse(cfg, ics.ExpandResult{Occurrences: []model.Occurrence{{
		Summary: "쉬는 날 어린이날", AllDay: true, Start: start, End: end,
	}}}, loc, start, end.AddDate(0, 0, 1))
	if len(result.Occurrences) != 0 || len(result.HolidayDates) != 1 || result.HolidayDates[0] != "2026-10-01" {
		t.Fatalf("holiday result = %+v", result)
	}
	cfg.ShowAllDay = true
	result = buildEventsResponse(cfg, ics.ExpandResult{Occurrences: []model.Occurrence{{
		Summary: "쉬는 날 어린이날", AllDay: true, Start: start, End: end,
	}}}, loc, start, end.AddDate(0, 0, 1))
	if len(result.Occurrences) != 1 || !result.Occurrences[0].Holiday || !result.Occurrences[0].HighlightRed {
		t.Fatalf("holiday event was not highlighted: %+v", result.Occurrences)
	}
}
