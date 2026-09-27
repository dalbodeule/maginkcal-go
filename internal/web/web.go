package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"epdcal/internal/battery"
	"epdcal/internal/config"
	"epdcal/internal/ics"
	appLog "epdcal/internal/log"
)

// Server provides HTTP APIs for configuration and schedule access.
type Server struct {
	current    atomic.Pointer[config.Config]
	configMu   sync.Mutex
	changed    chan struct{}
	configPath string
	debug      bool
	mux        *http.ServeMux

	// In-memory cache for /api/events responses to avoid redundant
	// fetch/parse/expand work on every HTTP request.
	eventsMu    sync.RWMutex
	eventsCache *eventsCache

	// In-memory cache for battery status, including temporary read failures.
	batteryReader battery.Reader
	icsClient     *http.Client
	batteryMu     sync.Mutex
	batteryCache  *batteryCache
	refresh       *RefreshManager
}

// embeddedStatic contains the exported Next.js static build.
//
// The directory structure under internal/web/static should mirror the
// output of `next export` (e.g. index.html, /calendar/index.html, etc).
//
//go:embed all:static
var embeddedStatic embed.FS

// NewServer constructs a new Server.
func NewServer(cfg *config.Config, debug bool) *Server {
	configPath := "/etc/epdcal/config.yaml"
	if debug {
		configPath = "./config.yaml"
	}
	s := &Server{
		configPath:    configPath,
		debug:         debug,
		mux:           http.NewServeMux(),
		batteryReader: battery.DefaultReader(),
		icsClient:     &http.Client{Timeout: 10 * time.Second},
		changed:       make(chan struct{}, 1),
	}
	s.current.Store(cfg)
	s.registerRoutes()
	return s
}

// Config returns an immutable snapshot of the settings currently in use.
func (s *Server) Config() *config.Config { return s.current.Load() }

func (s *Server) ConfigChanged() <-chan struct{} { return s.changed }

func (s *Server) SetConfigPath(path string) { s.configPath = path }

func (s *Server) SetRefreshManager(manager *RefreshManager) { s.refresh = manager }

// Handler returns the underlying http.Handler for this server.
func (s *Server) Handler() http.Handler {
	return s.basicAuthMiddleware(s.mux)
}

// basicAuthMiddleware wraps all handlers except /health with HTTP Basic Auth.
func (s *Server) basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /health 는 항상 무인증으로 노출한다.
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		cfg := s.Config()
		if cfg == nil || cfg.BasicAuth == nil || cfg.BasicAuth.Username == "" || cfg.BasicAuth.Password == "" {
			next.ServeHTTP(w, r)
			return
		}

		u, p, ok := r.BasicAuth()
		if !ok || !secureCompare(u, cfg.BasicAuth.Username) || !secureCompare(p, cfg.BasicAuth.Password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="EPDCal", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// secureCompare compares two strings in constant time.
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// StartServer binds the listener before returning so the first capture can
// safely request the calendar page immediately.
func StartServer(ctx context.Context, s *Server) (<-chan error, error) {
	listener, err := net.Listen("tcp", s.Config().Listen)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: s.Handler()}
	serverErrors := make(chan error, 1)
	appLog.Info("starting HTTP server", "listen", "http://"+s.Config().Listen, "debug", s.debug)
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErrors <- err
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			appLog.Error("HTTP server shutdown failed", err)
		}
	}()
	return serverErrors, nil
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/battery", s.handleBattery)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/ics/status", s.handleICSStatus)
	s.mux.HandleFunc("/api/refresh", s.handleRefresh)
	s.mux.HandleFunc("/app-config.js", s.handleAppConfigJS)
	s.mux.HandleFunc("/preview.png", s.handlePreview)

	// Static Next.js exported UI (embedded via Go 1.16+ embed.FS).
	// All non-/api/* and non-/preview.png paths fall back to this handler.
	s.mux.Handle("/", s.staticFileServer())

}

func (s *Server) InvalidateEventsCache() {
	s.eventsMu.Lock()
	s.eventsCache = nil
	s.eventsMu.Unlock()
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleBattery exposes current battery status (percent, voltage) for the Web UI.
//
// This endpoint returns an unknown status when the hardware is unavailable.
// Results, including failures, are cached briefly to avoid hammering I2C.
func (s *Server) handleBattery(w http.ResponseWriter, r *http.Request) {
	const batteryCacheTTL = 30 * time.Second
	w.Header().Set("Cache-Control", "no-store")
	s.batteryMu.Lock()
	if s.batteryCache != nil && time.Since(s.batteryCache.updatedAt) < batteryCacheTTL {
		resp := s.batteryCache.resp
		s.batteryMu.Unlock()
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp := batteryResponse{}
	status, err := s.batteryReader.Read(r.Context())
	if err != nil {
		appLog.Error("battery read failed", err)
	} else if status.Percent < 0 || status.Percent > 100 {
		appLog.Error("battery read failed", errors.New("invalid percentage"), "percent", status.Percent)
	} else {
		resp.Available = true
		resp.Percent = &status.Percent
		if status.VoltageMv > 0 {
			resp.VoltageMv = &status.VoltageMv
		}
	}
	s.batteryCache = &batteryCache{resp: resp, updatedAt: time.Now()}
	s.batteryMu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// handleAppConfigJS exposes a tiny runtime config payload for the static Web UI.
//
// The Next.js export is fully static, so browser-side code cannot directly read
// Go config values at build time. This endpoint bridges selected runtime values
// (currently default locale) into the browser before hydration.
func (s *Server) handleAppConfigJS(w http.ResponseWriter, _ *http.Request) {
	type browserRuntimeConfig struct {
		DefaultLocale string `json:"defaultLocale"`
	}

	runtimeCfg := browserRuntimeConfig{
		DefaultLocale: "ko",
	}
	if cfg := s.Config(); cfg != nil && cfg.DefaultLocale != "" {
		runtimeCfg.DefaultLocale = cfg.DefaultLocale
	}

	payload, err := json.Marshal(runtimeCfg)
	if err != nil {
		appLog.Error("failed to marshal browser runtime config", err)
		http.Error(w, "failed to build runtime config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("window.__EPDCAL_CONFIG__ = "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte(";\nif (window.__EPDCAL_CONFIG__ && window.__EPDCAL_CONFIG__.defaultLocale) { document.documentElement.lang = window.__EPDCAL_CONFIG__.defaultLocale; }\n"))
}

// staticFileServer returns an http.Handler that serves the embedded
// Next.js exported files from internal/web/static.
//
// Build-time expectation:
//   - Run `next build && next export` for the webui
//   - Copy the generated `out/` contents into `internal/web/static/`
//     before building the Go binary.
func (s *Server) staticFileServer() http.Handler {
	sub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		appLog.Error("failed to initialize embedded static filesystem", err)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static UI not available", http.StatusServiceUnavailable)
		})
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 절대 /api/* 요청은 정적 UI에서 서빙하지 않는다.
		// (API 핸들러가 없으면 404를 돌려주는 것이 맞고, HTML을 주면 안 됨)
		if path == "/api" || strings.HasPrefix(path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// /health, /preview.png 는 ServeMux 에 별도 핸들러가 등록되어 있어
		// 정상적인 경우 이 핸들러까지 도달하지 않는다.
		// 그 외 모든 경로는 Next 정적 빌드(embedded UI)로 서빙한다.
		fileServer.ServeHTTP(w, r)
	})
}

// handlePreview serves the last rendered PNG preview from disk.
// 경로 규칙은 cmd/epdcal/main.go 의 runCapturePipeline 과 동일하게 맞춘다:
//   - 기본:  /var/lib/epdcal/preview.png
//   - debug: ./cache/preview.png
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	previewPath := "/var/lib/epdcal/preview.png"
	if s.debug {
		previewPath = "./cache/preview.png"
	}

	// http.ServeFile 가 파일 존재/권한 문제에 대해 적절한 상태코드를 반환해 준다.
	// (존재하지 않으면 404, 기타 에러는 500 등)
	http.ServeFile(w, r, previewPath)
}

// eventsResponse is the JSON response shape for /api/events.
type eventsResponse struct {
	Occurrences     []occurrenceDTO `json:"occurrences"`
	HolidayDates    []string        `json:"holiday_dates"`
	TruncatedUIDs   []string        `json:"truncated_uids,omitempty"`
	RangeStart      time.Time       `json:"range_start"`
	RangeEnd        time.Time       `json:"range_end"`
	DisplayTimeZone string          `json:"display_timezone"`
	WeekStart       string          `json:"week_start"`
}

// eventsCache holds a cached /api/events response and its timestamp.
type eventsCache struct {
	key       string
	config    *config.Config
	resp      eventsResponse
	updatedAt time.Time
}

// batteryCache holds the last read result and its timestamp.
type batteryCache struct {
	resp      batteryResponse
	updatedAt time.Time
}

// batteryResponse is the JSON response shape for /api/battery.
type batteryResponse struct {
	Available bool `json:"available"`
	Percent   *int `json:"percent"`
	VoltageMv *int `json:"voltage_mv"`
}

// occurrenceDTO is a JSON-friendly view of occurrences.
type occurrenceDTO struct {
	SourceID     string    `json:"source_id"`
	UID          string    `json:"uid"`
	InstanceKey  string    `json:"instance_key"`
	Summary      string    `json:"summary"`
	Description  string    `json:"description"`
	Location     string    `json:"location"`
	AllDay       bool      `json:"all_day"`
	HighlightRed bool      `json:"highlight_red"`
	Holiday      bool      `json:"holiday"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
}

// handleEvents returns expanded occurrences for the configured ICS sources
// within a requested time window.
//
// GET /api/events?days=7&backfill=1
//   - days:     앞으로 몇 일을 볼 것인지 (기본 7)
//   - backfill: 과거 몇 일을 포함할지 (기본 1)
//
// 디스플레이 타임존은 config.Timezone 기준이며, 잘못된 Timezone 이면 time.Local 을 사용한다.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg := s.Config()

	// Parse query parameters.
	q := r.URL.Query()
	rawDays := q.Get("days")
	rawBackfill := q.Get("backfill")
	cacheKey := rawDays + "\x00" + rawBackfill

	days := parseIntDefault(rawDays, 7)
	if days <= 0 {
		days = 7
	}
	backfill := parseIntDefault(rawBackfill, 1)
	if backfill < 0 {
		backfill = 0
	}

	// Display timezone.
	loc := resolveLocationOrLocal(cfg.Timezone)

	// Small in-memory cache for expanded events. This avoids repeating
	// ICS fetch/parse/expand work on every HTTP request. The cache is
	// primarily a performance optimization for Web UI access; the main
	// refresh loop is still driven by cron in cmd/epdcal.
	const eventsCacheTTL = 30 * time.Second
	cacheNow := time.Now()

	s.eventsMu.RLock()
	ec := s.eventsCache
	s.eventsMu.RUnlock()
	if ec != nil && ec.key == cacheKey && ec.config == cfg && cacheNow.Sub(ec.updatedAt) < eventsCacheTTL {
		writeJSON(w, http.StatusOK, ec.resp)
		return
	}

	now := time.Now().In(loc)

	var rangeStart, rangeEnd time.Time
	if rawDays == "" && rawBackfill == "" {
		// 기본값: 이번 주 시작(week_start 설정에 따라 일/월)을 기준으로 35일 범위.
		rangeStart = startOfWeek(now, loc, cfg.WeekStart)
		rangeEnd = rangeStart.AddDate(0, 0, 35)

		// 로깅 편의를 위해 days/backfill 를 재계산한 개념값으로 덮어쓴다.
		// backfill 은 now 기준 과거 일 수, days 는 앞으로의 일 수로 본다.
		backfill = int(now.Sub(rangeStart).Hours() / 24)
		if backfill < 0 {
			backfill = 0
		}
		days = int(rangeEnd.Sub(now).Hours() / 24)
		if days <= 0 {
			days = 1
		}
	} else {
		// 사용자가 days/backfill 을 명시하면 기존 동작 유지: now 기준.
		rangeStart = now.AddDate(0, 0, -backfill)
		rangeEnd = now.AddDate(0, 0, days)
	}

	appLog.Info("api events request",
		"days", days,
		"backfill", backfill,
		"range_start", rangeStart.Format(time.RFC3339),
		"range_end", rangeEnd.Format(time.RFC3339),
		"timezone", cfg.Timezone,
	)

	// Build ICS sources from config.
	sources := make([]ics.Source, 0, len(cfg.ICS))
	for _, csrc := range cfg.ICS {
		if csrc.URL == "" {
			continue
		}
		id := csrc.ID
		if id == "" {
			if csrc.Name != "" {
				id = csrc.Name
			} else {
				id = csrc.URL
			}
		}
		sources = append(sources, ics.Source{
			ID:  id,
			URL: csrc.URL,
		})
	}

	if len(sources) == 0 {
		writeJSON(w, http.StatusOK, eventsResponse{
			Occurrences:     []occurrenceDTO{},
			TruncatedUIDs:   nil,
			RangeStart:      rangeStart,
			RangeEnd:        rangeEnd,
			DisplayTimeZone: loc.String(),
			WeekStart:       cfg.WeekStart,
		})
		return
	}

	// Choose cache dir: prod vs debug.
	const defaultCacheDir = "/var/lib/epdcal/ics-cache"
	cacheDir := defaultCacheDir
	if s.debug {
		cacheDir = "./cache/ics-cache"
	}

	fetcher := ics.NewFetcher(cacheDir)

	// Fetch ICS feeds.
	fetchResults, fetchErrs := fetcher.FetchAll(ctx, sources)
	if len(fetchErrs) > 0 {
		appLog.Error("api events: one or more ICS fetches failed", errorsAggregate(fetchErrs), "error_count", len(fetchErrs))
	}

	// Parse all ICS bodies into ParsedEvent list.
	parsedEvents := make([]ics.ParsedEvent, 0)
	for _, res := range fetchResults {
		events, err := ics.ParseICS(res.Source, res.Body)
		if err != nil {
			appLog.Error("api events: parse failed for source", err, "id", res.Source.ID)
			continue
		}
		parsedEvents = append(parsedEvents, events...)
	}

	// Expand into occurrences.
	expandCfg := ics.ExpandConfig{
		DisplayLocation:        loc,
		RangeStart:             rangeStart,
		RangeEnd:               rangeEnd,
		MaxOccurrencesPerEvent: 5000,
	}

	expandResult, err := ics.ExpandOccurrences(parsedEvents, expandCfg)
	if err != nil {
		appLog.Error("api events: expand failed", err)
		writeError(w, http.StatusInternalServerError, "failed to expand events")
		return
	}

	resp := buildEventsResponse(cfg, expandResult, loc, rangeStart, rangeEnd)

	// Update in-memory cache for subsequent requests.
	s.eventsMu.Lock()
	s.eventsCache = &eventsCache{
		key:       cacheKey,
		config:    cfg,
		resp:      resp,
		updatedAt: time.Now(),
	}
	s.eventsMu.Unlock()

	writeJSON(w, http.StatusOK, resp)
}

func buildEventsResponse(cfg *config.Config, expanded ics.ExpandResult, loc *time.Location, rangeStart, rangeEnd time.Time) eventsResponse {
	dtos := make([]occurrenceDTO, 0, len(expanded.Occurrences))
	holidayDates := make(map[string]struct{})
	for _, occ := range expanded.Occurrences {
		holiday := isHoliday(occ.Summary, cfg.HolidayPrefixes)
		if holiday {
			for _, key := range holidayDateKeys(occ.Start, occ.End, loc, rangeStart, rangeEnd) {
				holidayDates[key] = struct{}{}
			}
		}
		if occ.AllDay && !cfg.ShowAllDay {
			continue
		}
		dtos = append(dtos, occurrenceDTO{
			SourceID:     occ.SourceID,
			UID:          occ.UID,
			InstanceKey:  occ.InstanceKey,
			Summary:      occ.Summary,
			Description:  occ.Description,
			Location:     occ.Location,
			AllDay:       occ.AllDay,
			HighlightRed: holiday || shouldHighlightRed(occ.Summary, occ.Description, cfg.HighlightRed),
			Holiday:      holiday,
			Start:        occ.Start,
			End:          occ.End,
		})
	}
	dates := make([]string, 0, len(holidayDates))
	for key := range holidayDates {
		dates = append(dates, key)
	}
	sort.Strings(dates)

	return eventsResponse{
		Occurrences:     dtos,
		HolidayDates:    dates,
		TruncatedUIDs:   expanded.TruncatedEvents,
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		DisplayTimeZone: loc.String(),
		WeekStart:       cfg.WeekStart,
	}
}

func shouldHighlightRed(summary, description string, keywords []string) bool {
	text := strings.ToLower(summary + " " + description)
	for _, keyword := range keywords {
		needle := strings.ToLower(strings.TrimSpace(keyword))
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func isHoliday(summary string, prefixes []string) bool {
	summary = strings.TrimSpace(summary)
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && strings.HasPrefix(summary, prefix) {
			return true
		}
	}
	return false
}

func holidayDateKeys(start, end time.Time, loc *time.Location, rangeStart, rangeEnd time.Time) []string {
	start = start.In(loc)
	end = end.In(loc)
	if !end.After(start) {
		end = start.Add(time.Nanosecond)
	}
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	firstRangeDay := rangeStart.In(loc)
	firstRangeDay = time.Date(firstRangeDay.Year(), firstRangeDay.Month(), firstRangeDay.Day(), 0, 0, 0, 0, loc)
	if day.Before(firstRangeDay) {
		day = firstRangeDay
	}
	keys := make([]string, 0, 1)
	for count := 0; day.Before(rangeEnd) && day.Before(end) && count < 366; count++ {
		keys = append(keys, day.Format("2006-01-02"))
		day = day.AddDate(0, 0, 1)
	}
	return keys
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func resolveLocationOrLocal(name string) *time.Location {
	if name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		appLog.Error("failed to load timezone; falling back to local", err, "name", name)
		return time.Local
	}
	return loc
}

// startOfWeek returns the start of the week (00:00 in loc) for the given time.
// weekStart is a config string (e.g. "sunday" or "monday"). Any 값이 "sunday"
// 가 아니면 모두 Monday 시작으로 간주한다.
func startOfWeek(t time.Time, loc *time.Location, weekStart string) time.Time {
	if loc == nil {
		loc = time.Local
	}

	_tt := t.In(loc)
	weekday := int(_tt.Weekday()) // Sunday=0, Monday=1, ...

	startIndex := 1 // Monday
	if strings.ToLower(weekStart) == "sunday" {
		startIndex = 0
	}

	// 현재 요일에서 주 시작 요일까지 되돌아가는 일 수.
	delta := (7 + weekday - startIndex) % 7

	midnightToday := time.Date(_tt.Year(), _tt.Month(), _tt.Day(), 0, 0, 0, 0, loc)
	return midnightToday.AddDate(0, 0, -delta)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		appLog.Error("failed to write JSON response", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	type errResp struct {
		Error string `json:"error"`
	}
	writeJSON(w, status, errResp{Error: msg})
}

// errorsAggregate is similar to the helper in cmd/epdcal/main.go.
// TODO: deduplicate in a shared internal package.
func errorsAggregate(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	var b strings.Builder
	for i, e := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(e.Error())
	}
	return errors.New(b.String())
}
