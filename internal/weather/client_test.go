package weather

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"epdcal/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func coordinate(value float64) *float64 { return &value }

func TestFetchCurrentWeather25(t *testing.T) {
	t.Setenv("EPDCAL_OPENWEATHER_API_KEY", "test-key")
	requestCount := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestCount++
		if r.URL.Host != "api.openweathermap.org" || r.URL.Path != "/data/2.5/weather" {
			return nil, fmt.Errorf("unexpected endpoint %q", r.URL.String())
		}
		if r.URL.Query().Get("appid") != "test-key" || r.URL.Query().Get("units") != "metric" {
			return nil, fmt.Errorf("missing API parameters")
		}
		if _, ok := r.URL.Query()["lang"]; ok {
			return nil, fmt.Errorf("unexpected lang parameter")
		}
		if r.URL.Query().Get("lat") != "37.5" || r.URL.Query().Get("lon") != "127" {
			return nil, fmt.Errorf("unexpected coordinates: lat=%q lon=%q", r.URL.Query().Get("lat"), r.URL.Query().Get("lon"))
		}
		body := `{"weather":[{"description":"구름 조금"}],"main":{"temp":18.6}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	got, err := Fetch(context.Background(), client, config.WeatherConfig{Enabled: true, Location: "Seoul", Latitude: coordinate(37.5), Longitude: coordinate(127)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "구름 조금" || got.CurrentTemp == nil || *got.CurrentTemp != 18.6 || len(got.Days) != 0 {
		t.Fatalf("unexpected forecast: %+v", got)
	}
	if requestCount != 1 {
		t.Fatalf("made %d provider requests, want one current-weather request", requestCount)
	}
}

func TestFetchRejectsUnusableCurrentResponse(t *testing.T) {
	t.Setenv("EPDCAL_OPENWEATHER_API_KEY", "test-key")
	for _, body := range []string{
		`{"weather":[],"main":{"temp":20}}`,
		`{"weather":[{"description":"Clear"}]}`,
		`{"weather":[{"description":"Clear"}],"main":{"temp":"not-a-number"}}`,
		`{"weather":[{"description":"Clear"}],"main":{"temp":20}} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			_, err := Fetch(context.Background(), client, config.WeatherConfig{Enabled: true, Location: "Seoul", Latitude: coordinate(37.5), Longitude: coordinate(127)})
			if err == nil {
				t.Fatal("unusable provider response accepted")
			}
		})
	}
}

func TestFetchRejectsInvalidCoordinatesBeforeRequest(t *testing.T) {
	t.Setenv("EPDCAL_OPENWEATHER_API_KEY", "test-key")
	for _, tt := range []struct {
		name string
		lat  float64
		lon  float64
	}{
		{name: "latitude out of range", lat: 90.000001, lon: 127},
		{name: "longitude out of range", lat: 37, lon: -180.000001},
		{name: "NaN", lat: math.NaN(), lon: 127},
		{name: "infinite", lat: 37, lon: math.Inf(1)},
		{name: "too many decimals", lat: 37.1234567, lon: 127},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				called = true
				return nil, fmt.Errorf("unexpected request")
			})}
			_, err := Fetch(context.Background(), client, config.WeatherConfig{Enabled: true, Location: "Seoul", Latitude: coordinate(tt.lat), Longitude: coordinate(tt.lon)})
			if err == nil || called {
				t.Fatalf("invalid coordinates: err=%v request-called=%v", err, called)
			}
		})
	}
}

func TestFetchDoesNotLeakAPIKeyOnTransportError(t *testing.T) {
	const apiKey = "private-test-key"
	t.Setenv("EPDCAL_OPENWEATHER_API_KEY", apiKey)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s", r.URL.String())
	})}
	_, err := Fetch(context.Background(), client, config.WeatherConfig{Enabled: true, Location: "Seoul", Latitude: coordinate(37.5), Longitude: coordinate(127)})
	if err == nil {
		t.Fatal("expected request failure")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("API key leaked in request error: %v", err)
	}
}
