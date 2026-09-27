package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"epdcal/internal/config"
)

func TestConfigAPIStoresEditsAndPreservesHiddenFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := config.DefaultConfig()
	initial.Rotation = 270
	initial.DefaultLocale = "en"
	initial.ICS = []config.ICSConfig{{ID: "home", Name: "Home", URL: "https://example.com/home.ics"}}
	if err := config.Save(path, initial); err != nil {
		t.Fatal(err)
	}
	s := NewServer(initial, true)
	s.configPath = path

	get := httptest.NewRecorder()
	s.Handler().ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d: %s", get.Code, get.Body.String())
	}
	var input editableConfig
	if err := json.Unmarshal(get.Body.Bytes(), &input); err != nil {
		t.Fatal(err)
	}
	if input.ICS[0].Name != "Home" {
		t.Fatalf("ICS name was lost in GET: %+v", input.ICS[0])
	}
	input.Refresh = "0 * * * *"
	input.HighlightRedKeywords = []string{"urgent"}
	input.BasicAuth = editableBasicAuth{Enabled: true, Username: "admin", Password: "secret"}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	post := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(post, req)
	if post.Code != http.StatusOK {
		t.Fatalf("POST status = %d: %s", post.Code, post.Body.String())
	}

	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshCron != input.Refresh || saved.Rotation != 270 || saved.DefaultLocale != "en" ||
		len(saved.HighlightRed) != 1 || saved.HighlightRed[0] != "urgent" ||
		len(saved.ICS) != 1 || saved.ICS[0].Name != "Home" ||
		saved.BasicAuth == nil || saved.BasicAuth.Username != "admin" {
		t.Fatalf("saved config = %+v", saved)
	}
	if initial.BasicAuth != nil {
		t.Fatal("running config changed before restart")
	}
}

func TestConfigAPIRejectsInvalidSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := config.DefaultConfig()
	if err := config.Save(path, initial); err != nil {
		t.Fatal(err)
	}
	s := NewServer(initial, true)
	s.configPath = path
	input := editableFromConfig(initial)
	input.Refresh = "not a cron expression"
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RefreshCron != initial.RefreshCron {
		t.Fatalf("invalid schedule was saved: %q", saved.RefreshCron)
	}
}
