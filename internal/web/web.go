package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"epdcal/internal/battery"
	"epdcal/internal/config"
	"epdcal/internal/ics"
	appLog "epdcal/internal/log"
)

// Server provides HTTP APIs for configuration and schedule access.
// 현재는 /health 와 /api/events 두 개의 엔드포인트만 구현한다.
type Server struct {
	cfg   *config.Config
	debug bool
	mux   *http.ServeMux
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
	s := &Server{
		cfg:   cfg,
		debug: debug,
		mux:   http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the underlying http.Handler for this server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// StartServer starts an HTTP server bound to cfg.Listen and serves
// API + (추후) 정적 파일. ctx 가 cancel 되면 graceful shutdown 할 수 있도록
// Shutdown 로직은 main 쪽에서 http.Server 래핑 시 구현하는 것을 권장한다.
// 이 함수는 API 핸들러 구현에 포커스하기 위해 간단한 ListenAndServe 만 제공한다.
func StartServer(_ context.Context, cfg *config.Config, debug bool) error {
	s := NewServer(cfg, debug)
	appLog.Info("starting HTTP server", "listen", "http://"+cfg.Listen, "debug", debug)
	return http.ListenAndServe(cfg.Listen, s.Handler())
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/battery", s.handleBattery)
	s.mux.HandleFunc("/preview.png", s.handlePreview)

	// Static Next.js exported UI (embedded via Go 1.16+ embed.FS).
	// All non-/api/* and non-/preview.png paths fall back to this handler.
	s.mux.Handle("/", s.staticFileServer())

	// 추후:
	// - /api/config
	// - /api/refresh
	// - /api/render
	// - 정적 파일 서빙(Next.js export 결과) 등을 여기에 추가 예정.
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleBattery exposes current battery status (percent, voltage) for the Web UI.
//
// For now this endpoint uses battery.DefaultReader(), which is backed by a
// mock implementation returning a random percentage. Later, DefaultReader
// can be wired to a PiSugar3 I2C-based Reader on Raspberry Pi.
func (s *Server) handleBattery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	br := battery.DefaultReader()
	if br == nil {
		writeError(w, http.StatusInternalServerError, "battery reader unavailable")
		return
	}

	status, err := br.Read(ctx)
	if err != nil {
		appLog.Error("battery read failed", err)
		writeError(w, http.StatusInternalServerError, "failed to read battery")
		return
	}

	type batteryResponse struct {
		Percent   int `json:"percent"`
		VoltageMv int `json:"voltage_mv"`
	}

	resp := batteryResponse{
		Percent:   status.Percent,
		VoltageMv: status.VoltageMv,
	}
	writeJSON(w, http.StatusOK, resp)
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

// handlePreview will later serve the last rendered PNG preview from disk.
// For now it is a stub to ensure /preview.png is not caught by the static handler.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "preview not implemented", http.StatusNotImplemented)
}

// eventsResponse is the JSON response shape for /api/events.
type eventsResponse struct {
	Occurrences     []occurrenceDTO `json:"occurrences"`
	TruncatedUIDs   []string        `json:"truncated_uids,omitempty"`
	RangeStart      time.Time       `json:"range_start"`
	RangeEnd        time.Time       `json:"range_end"`
	DisplayTimeZone string          `json:"display_timezone"`
	WeekStart       string          `json:"week_start"`
}

// occurrenceDTO is a JSON-friendly view of occurrences.
type occurrenceDTO struct {
	SourceID    string    `json:"source_id"`
	UID         string    `json:"uid"`
	InstanceKey string    `json:"instance_key"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	AllDay      bool      `json:"all_day"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
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

	// Parse query parameters.
	q := r.URL.Query()
	days := parseIntDefault(q.Get("days"), 7)
	if days <= 0 {
		days = 7
	}
	backfill := parseIntDefault(q.Get("backfill"), 1)
	if backfill < 0 {
		backfill = 0
	}

	// Display timezone.
	loc := resolveLocationOrLocal(s.cfg.Timezone)

	now := time.Now().In(loc)
	rangeStart := now.AddDate(0, 0, -backfill)
	rangeEnd := now.AddDate(0, 0, days)

	appLog.Info("api events request",
		"days", days,
		"backfill", backfill,
		"range_start", rangeStart.Format(time.RFC3339),
		"range_end", rangeEnd.Format(time.RFC3339),
		"timezone", s.cfg.Timezone,
	)

	// Build ICS sources from config.
	sources := make([]ics.Source, 0, len(s.cfg.ICS))
	for _, csrc := range s.cfg.ICS {
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
			WeekStart:       s.cfg.WeekStart,
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

	// Convert to DTO.
	dtos := make([]occurrenceDTO, 0, len(expandResult.Occurrences))
	for _, occ := range expandResult.Occurrences {
		dtos = append(dtos, occurrenceDTO{
			SourceID:    occ.SourceID,
			UID:         occ.UID,
			InstanceKey: occ.InstanceKey,
			Summary:     occ.Summary,
			Description: occ.Description,
			Location:    occ.Location,
			AllDay:      occ.AllDay,
			Start:       occ.Start,
			End:         occ.End,
		})
	}

	resp := eventsResponse{
		Occurrences:     dtos,
		TruncatedUIDs:   expandResult.TruncatedEvents,
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		DisplayTimeZone: loc.String(),
		WeekStart:       s.cfg.WeekStart,
	}

	writeJSON(w, http.StatusOK, resp)
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
