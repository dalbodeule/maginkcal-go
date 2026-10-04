package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"epdcal/internal/config"
)

type Forecast struct {
	Location    string   `json:"location"`
	Current     string   `json:"current"`
	CurrentTemp *float64 `json:"current_temp,omitempty"`
	Days        []Day    `json:"days"`
}
type Day struct {
	Date        string  `json:"date"`
	Description string  `json:"description"`
	Low         float64 `json:"low"`
	High        float64 `json:"high"`
	Sunrise     string  `json:"sunrise"`
	Sunset      string  `json:"sunset"`
}
type apiResponse struct {
	Main struct {
		Temp json.RawMessage `json:"temp"`
	} `json:"main"`
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
}

func Fetch(ctx context.Context, client *http.Client, cfg config.WeatherConfig) (Forecast, error) {
	if !cfg.Enabled {
		return Forecast{}, errors.New("weather: fetching is disabled in config")
	}
	if err := config.ValidateWeatherConfig(cfg); err != nil {
		return Forecast{}, fmt.Errorf("weather: invalid configuration: %w", err)
	}
	key := os.Getenv("EPDCAL_OPENWEATHER_API_KEY")
	if key == "" {
		return Forecast{}, fmt.Errorf("weather: EPDCAL_OPENWEATHER_API_KEY is not set")
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	base := "https://api.openweathermap.org/data/2.5/weather"
	query := url.Values{"lat": {strconv.FormatFloat(*cfg.Latitude, 'f', -1, 64)}, "lon": {strconv.FormatFloat(*cfg.Longitude, 'f', -1, 64)}, "units": {"metric"}, "appid": {key}}
	var current apiResponse
	if err := fetch(ctx, client, base+"?"+query.Encode(), &current); err != nil {
		return Forecast{}, err
	}
	if len(current.Weather) == 0 || strings.TrimSpace(current.Weather[0].Description) == "" {
		return Forecast{}, fmt.Errorf("weather: current endpoint returned no usable weather record")
	}
	var temperature float64
	if err := json.Unmarshal(current.Main.Temp, &temperature); err != nil || math.IsNaN(temperature) || math.IsInf(temperature, 0) {
		return Forecast{}, fmt.Errorf("weather: current endpoint returned an invalid temperature")
	}
	out := Forecast{Location: strings.TrimSpace(cfg.Location), Current: current.Weather[0].Description, CurrentTemp: &temperature, Days: []Day{}}
	return out, nil
}

func fetch(ctx context.Context, client *http.Client, endpoint string, out *apiResponse) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return safeRequestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("weather: provider returned HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("weather: invalid provider response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("weather: provider response contains trailing data")
	}
	return nil
}

func safeRequestError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("weather: request timed out")
	case errors.Is(err, context.Canceled):
		return errors.New("weather: request canceled")
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return fmt.Errorf("weather: DNS lookup failed for %s", dnsErr.Name)
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("weather: network operation %s failed", opErr.Op)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("weather: request failed (%T)", urlErr.Err)
	}
	return fmt.Errorf("weather: request failed (%T)", err)
}
