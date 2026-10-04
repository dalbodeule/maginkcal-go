package config

import (
	"errors"
	"math"
	"strings"
)

const MaxWeatherCoordinateDecimalPlaces = 6

// ValidateWeatherConfig checks coordinates before they are persisted or sent
// to the provider. Six decimal places is already well below a meter on Earth.
func ValidateWeatherConfig(cfg WeatherConfig) error {
	if cfg.Enabled && strings.TrimSpace(cfg.Location) == "" {
		return errors.New("weather location is required when weather is enabled")
	}
	if cfg.Latitude == nil && cfg.Enabled {
		return errors.New("weather latitude is required when weather is enabled")
	}
	if cfg.Latitude != nil && !validCoordinate(*cfg.Latitude, -90, 90) {
		return errors.New("weather latitude must be finite, between -90 and 90, and have at most 6 decimal places")
	}
	if cfg.Longitude == nil && cfg.Enabled {
		return errors.New("weather longitude is required when weather is enabled")
	}
	if cfg.Longitude != nil && !validCoordinate(*cfg.Longitude, -180, 180) {
		return errors.New("weather longitude must be finite, between -180 and 180, and have at most 6 decimal places")
	}
	return nil
}

func validCoordinate(value, min, max float64) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < min || value > max {
		return false
	}
	scale := math.Pow10(MaxWeatherCoordinateDecimalPlaces)
	rounded := math.Round(value*scale) / scale
	return math.Abs(value-rounded) <= 1e-12
}
