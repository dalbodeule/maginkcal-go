package web

import (
	"context"
	"net/http"
	"time"

	"epdcal/internal/weather"
)

type weatherResponse struct {
	Available bool             `json:"available"`
	Forecast  weather.Forecast `json:"forecast"`
	Error     string           `json:"error,omitempty"`
}

func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	cfg := s.Config()
	if cfg == nil || !cfg.Weather.Enabled {
		writeJSON(w, http.StatusOK, weatherResponse{})
		return
	}
	s.weatherMu.Lock()
	defer s.weatherMu.Unlock()
	cacheTTL := 30 * time.Minute
	if s.weatherErr != "" {
		cacheTTL = 5 * time.Minute
	}
	if s.weatherConfig == cfg && time.Since(s.weatherCachedAt) < cacheTTL {
		writeJSON(w, http.StatusOK, weatherResponse{Available: s.weatherErr == "", Forecast: s.weatherData, Error: s.weatherErr})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	data, err := weather.Fetch(ctx, nil, cfg.Weather)
	s.weatherConfig = cfg
	s.weatherCachedAt = time.Now()
	s.weatherData = data
	s.weatherErr = ""
	if err != nil {
		s.weatherErr = err.Error()
		writeJSON(w, http.StatusOK, weatherResponse{Error: s.weatherErr})
		return
	}
	writeJSON(w, http.StatusOK, weatherResponse{Available: true, Forecast: data})
}
