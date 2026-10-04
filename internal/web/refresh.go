package web

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	appLog "epdcal/internal/log"
)

const manualRefreshCooldown = 5 * time.Minute

type RefreshStatus struct {
	Running       bool       `json:"running"`
	LastStartedAt *time.Time `json:"last_started_at,omitempty"`
	LastEndedAt   *time.Time `json:"last_ended_at,omitempty"`
	NextAllowedAt *time.Time `json:"next_allowed_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

// RefreshManager serializes all render/display work. Only manual runs consume
// the cooldown; cron runs may proceed whenever the pipeline is idle.
type RefreshManager struct {
	mu     sync.Mutex
	status RefreshStatus
	run    func(context.Context) error
	ctx    context.Context
	done   chan struct{}
}

func NewRefreshManager(ctx context.Context, run func(context.Context) error) *RefreshManager {
	return &RefreshManager{run: run, ctx: ctx}
}

func (m *RefreshManager) Status() RefreshStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *RefreshManager) start(manual bool) (RefreshStatus, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if m.ctx.Err() != nil {
		return m.status, http.StatusServiceUnavailable
	}
	if m.status.Running {
		return m.status, http.StatusConflict
	}
	if manual && m.status.NextAllowedAt != nil && now.Before(*m.status.NextAllowedAt) {
		return m.status, http.StatusTooManyRequests
	}
	m.status.Running = true
	m.done = make(chan struct{})
	m.status.LastStartedAt = &now
	m.status.LastEndedAt = nil
	m.status.LastError = ""
	if manual {
		next := now.Add(manualRefreshCooldown)
		m.status.NextAllowedAt = &next
	}
	return m.status, http.StatusAccepted
}

func (m *RefreshManager) finish(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.status.Running = false
	m.status.LastEndedAt = &now
	if err != nil {
		m.status.LastError = err.Error()
	}
	close(m.done)
}

func (m *RefreshManager) WaitIdle() {
	m.mu.Lock()
	done := m.done
	m.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (m *RefreshManager) RunScheduled(ctx context.Context) (bool, error) {
	_, status := m.start(false)
	if status != http.StatusAccepted {
		return false, nil
	}
	err := m.run(ctx)
	m.finish(err)
	return true, err
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.refresh == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "refresh unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.refresh.Status())
	case http.MethodPost:
		if r.Header.Get("Content-Type") != "application/json" {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "application/json required"})
			return
		}
		status, code := s.refresh.start(true)
		if code == http.StatusTooManyRequests && status.NextAllowedAt != nil {
			seconds := int(time.Until(*status.NextAllowedAt).Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
		}
		if code == http.StatusAccepted {
			go func() {
				err := s.refresh.run(s.refresh.ctx)
				if err != nil {
					appLog.Error("manual refresh failed", err)
				}
				s.refresh.finish(err)
			}()
		}
		writeJSON(w, code, status)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
