package web

import (
	"io"
	"net/http"

	"epdcal/internal/config"
	"epdcal/internal/ics"
)

type icsStatus struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	HTTPStatus int    `json:"http_status,omitempty"`
	EventCount int    `json:"event_count,omitempty"`
}

// This checks saved feeds over the network without accepting an arbitrary URL
// from the request or silently falling back to a stale ICS cache.
func (s *Server) handleICSStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg, err := config.Load(s.configPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config")
		return
	}
	results := make([]icsStatus, 0, len(cfg.ICS))
	for _, source := range cfg.ICS {
		results = append(results, checkICS(r, s.icsClient, source))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": results})
}

func checkICS(r *http.Request, client *http.Client, source config.ICSConfig) icsStatus {
	status := icsStatus{ID: source.ID, State: "network_error"}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, source.URL, nil)
	if err != nil {
		return status
	}
	resp, err := client.Do(req)
	if err != nil {
		return status
	}
	defer resp.Body.Close()
	status.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		status.State = "http_error"
		return status
	}
	const maxICSBytes = 10 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxICSBytes+1))
	if err != nil {
		return status
	}
	if len(body) > maxICSBytes {
		status.State = "too_large"
		return status
	}
	events, err := ics.ParseICS(ics.Source{ID: source.ID, URL: source.URL}, body)
	if err != nil {
		status.State = "invalid_ics"
		return status
	}
	status.State = "ok"
	status.EventCount = len(events)
	return status
}
