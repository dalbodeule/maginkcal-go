package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"epdcal/internal/battery"
)

type batteryReaderFunc func(context.Context) (battery.Status, error)

func (f batteryReaderFunc) Read(ctx context.Context) (battery.Status, error) {
	return f(ctx)
}

func TestBatteryUnavailableAndRetry(t *testing.T) {
	s := NewServer(nil, true)
	reads := 0
	s.batteryReader = batteryReaderFunc(func(context.Context) (battery.Status, error) {
		reads++
		if reads == 1 {
			return battery.Status{}, errors.New("i2c unavailable")
		}
		return battery.Status{Percent: 42, VoltageMv: 3850}, nil
	})

	request := func() batteryResponse {
		t.Helper()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/battery", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", got)
		}
		var resp batteryResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if resp := request(); resp.Available || resp.Percent != nil || resp.VoltageMv != nil {
		t.Fatalf("unavailable response = %+v", resp)
	}
	if resp := request(); resp.Available || reads != 1 {
		t.Fatalf("failure cache missed: response = %+v, reads = %d", resp, reads)
	}

	s.batteryCache.updatedAt = time.Now().Add(-time.Minute)
	if resp := request(); !resp.Available || resp.Percent == nil || *resp.Percent != 42 || reads != 2 {
		t.Fatalf("retry response = %+v, reads = %d", resp, reads)
	}
}

func TestBatteryInvalidPercentIsUnavailable(t *testing.T) {
	s := NewServer(nil, true)
	s.batteryReader = batteryReaderFunc(func(context.Context) (battery.Status, error) {
		return battery.Status{Percent: 255, VoltageMv: 3800}, nil
	})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/battery", nil))
	var resp batteryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Available || resp.Percent != nil {
		t.Fatalf("invalid percentage response = %+v", resp)
	}
}

func TestBatteryZeroPercentWithUnknownVoltage(t *testing.T) {
	s := NewServer(nil, true)
	s.batteryReader = batteryReaderFunc(func(context.Context) (battery.Status, error) {
		return battery.Status{Percent: 0}, nil
	})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/battery", nil))
	var resp batteryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Available || resp.Percent == nil || *resp.Percent != 0 || resp.VoltageMv != nil {
		t.Fatalf("zero percentage response = %+v", resp)
	}
}
