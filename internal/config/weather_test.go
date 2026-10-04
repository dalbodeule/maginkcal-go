package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func weatherCoordinate(value float64) *float64 { return &value }

func TestValidateWeatherConfigCoordinateBoundsAndPrecision(t *testing.T) {
	valid := []WeatherConfig{
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(-90), Longitude: weatherCoordinate(-180)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(90), Longitude: weatherCoordinate(180)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(37.566535), Longitude: weatherCoordinate(126.978000)},
		{Enabled: true, Location: "Null Island", Latitude: weatherCoordinate(0), Longitude: weatherCoordinate(0)},
	}
	for _, cfg := range valid {
		if err := ValidateWeatherConfig(cfg); err != nil {
			t.Errorf("valid coordinates rejected (%v, %v): %v", cfg.Latitude, cfg.Longitude, err)
		}
	}

	invalid := []WeatherConfig{
		{Enabled: true, Location: "Seoul", Latitude: nil, Longitude: weatherCoordinate(127)},
		{Enabled: false, Latitude: weatherCoordinate(math.NaN())},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(math.NaN()), Longitude: weatherCoordinate(127)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(math.Inf(1)), Longitude: weatherCoordinate(127)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(-90.000001), Longitude: weatherCoordinate(127)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(37), Longitude: weatherCoordinate(180.000001)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(37.1234567), Longitude: weatherCoordinate(127)},
		{Enabled: true, Location: "Seoul", Latitude: weatherCoordinate(37.1234560005), Longitude: weatherCoordinate(127)},
		{Enabled: true, Location: " ", Latitude: weatherCoordinate(37), Longitude: weatherCoordinate(127)},
	}
	for _, cfg := range invalid {
		if err := ValidateWeatherConfig(cfg); err == nil {
			t.Errorf("invalid coordinates accepted: %+v", cfg)
		}
	}
}

func TestLoadRejectsInvalidEnabledWeatherCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name string
		lat  string
		lon  string
	}{
		{name: "yaml nan", lat: ".nan", lon: "127"},
		{name: "out of range", lat: "91", lon: "127"},
		{name: "excess precision", lat: "37.1234567", lon: "127"},
		{name: "missing latitude", lat: "", lon: "127"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			data := []byte("weather:\n  enabled: true\n  location: Seoul\n  latitude: " + tc.lat + "\n  longitude: " + tc.lon + "\n")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("invalid weather coordinates loaded")
			}
		})
	}
}
