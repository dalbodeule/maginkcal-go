package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"epdcal/internal/config"
	appLog "epdcal/internal/log"
)

type editableBasicAuth struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type editableConfig struct {
	Listen               string             `json:"listen"`
	Timezone             string             `json:"timezone"`
	Refresh              string             `json:"refresh"`
	HorizonDays          int                `json:"horizon_days"`
	ShowAllDay           bool               `json:"show_all_day"`
	HighlightRedKeywords []string           `json:"highlight_red_keywords"`
	HolidayPrefixes      []string           `json:"holiday_prefixes"`
	WeekStart            string             `json:"week_start"`
	ICS                  []config.ICSConfig `json:"ics"`
	BasicAuth            editableBasicAuth  `json:"basic_auth"`
}

func editableFromConfig(cfg *config.Config) editableConfig {
	result := editableConfig{
		Listen:               cfg.Listen,
		Timezone:             cfg.Timezone,
		Refresh:              cfg.RefreshCron,
		HorizonDays:          cfg.HorizonDays,
		ShowAllDay:           cfg.ShowAllDay,
		HighlightRedKeywords: cfg.HighlightRed,
		HolidayPrefixes:      cfg.HolidayPrefixes,
		WeekStart:            cfg.WeekStart,
		ICS:                  cfg.ICS,
	}
	if cfg.BasicAuth != nil {
		result.BasicAuth = editableBasicAuth{
			Enabled:  cfg.BasicAuth.Username != "" && cfg.BasicAuth.Password != "",
			Username: cfg.BasicAuth.Username,
			Password: cfg.BasicAuth.Password,
		}
	}
	return result
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.Load(s.configPath)
		if err != nil {
			appLog.Error("failed to load config for API", err)
			writeError(w, http.StatusInternalServerError, "failed to load config")
			return
		}
		writeJSON(w, http.StatusOK, editableFromConfig(cfg))
	case http.MethodPost:
		s.saveConfig(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	var input editableConfig
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid config JSON")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "config JSON must contain one object")
		return
	}
	if err := validateEditableConfig(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Merge with the on-disk config so fields not exposed by the UI survive.
	cfg, err := config.Load(s.configPath)
	if err != nil {
		appLog.Error("failed to load config before saving", err)
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	cfg.Timezone = input.Timezone
	cfg.RefreshCron = input.Refresh
	cfg.HorizonDays = input.HorizonDays
	cfg.ShowAllDay = input.ShowAllDay
	cfg.HighlightRed = input.HighlightRedKeywords
	cfg.HolidayPrefixes = input.HolidayPrefixes
	cfg.WeekStart = input.WeekStart
	cfg.ICS = input.ICS
	if input.BasicAuth.Enabled {
		cfg.BasicAuth = &config.BasicAuthConfig{
			Username: input.BasicAuth.Username,
			Password: input.BasicAuth.Password,
		}
	} else {
		cfg.BasicAuth = nil
	}
	if err := config.Save(s.configPath, cfg); err != nil {
		appLog.Error("failed to save config from API", err)
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	runtimeCfg := *cfg
	if active := s.Config(); active != nil {
		// An explicit --listen override remains authoritative until restart.
		runtimeCfg.Listen = active.Listen
	}
	s.current.Store(&runtimeCfg)
	s.InvalidateEventsCache()
	select {
	case s.changed <- struct{}{}:
	default:
	}
	writeJSON(w, http.StatusOK, editableFromConfig(cfg))
}

func validateEditableConfig(input editableConfig) error {
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return errors.New("invalid timezone")
	}
	if _, err := cron.ParseStandard(input.Refresh); err != nil {
		return errors.New("invalid refresh schedule")
	}
	if input.HorizonDays < 1 || input.HorizonDays > 365 {
		return errors.New("horizon_days must be between 1 and 365")
	}
	if input.WeekStart != "monday" && input.WeekStart != "sunday" {
		return errors.New("week_start must be monday or sunday")
	}
	if len(input.HolidayPrefixes) > 32 {
		return errors.New("too many holiday prefixes")
	}
	for _, prefix := range input.HolidayPrefixes {
		if strings.TrimSpace(prefix) == "" || len(prefix) > 128 {
			return errors.New("holiday prefixes must be nonempty and at most 128 bytes")
		}
	}
	ids := make(map[string]bool, len(input.ICS))
	for _, source := range input.ICS {
		if source.ID == "" || ids[source.ID] {
			return errors.New("ICS source IDs must be nonempty and unique")
		}
		ids[source.ID] = true
		u, err := url.Parse(source.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("ICS URLs must be HTTP or HTTPS")
		}
	}
	if input.BasicAuth.Enabled && (strings.TrimSpace(input.BasicAuth.Username) == "" || input.BasicAuth.Password == "") {
		return errors.New("basic auth needs a username and password")
	}
	return nil
}
