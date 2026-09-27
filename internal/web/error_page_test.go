package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"epdcal/internal/config"
)

func TestUnauthorizedBrowserRequestRedirectsToLogin(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BasicAuth = &config.BasicAuthConfig{Username: "admin", Password: "secret"}
	s := NewServer(cfg, true)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config", nil))
	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/login?next=") {
		t.Fatalf("status/location = %d/%q", w.Code, w.Header().Get("Location"))
	}
}

func TestWebLoginCreatesSession(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BasicAuth = &config.BasicAuthConfig{Username: "admin", Password: "secret"}
	s := NewServer(cfg, true)
	login := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=admin&password=secret&next=%2Fconfig"))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, login)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/config" {
		t.Fatalf("login response = %d/%q", w.Code, w.Header().Get("Location"))
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("session cookie = %+v", cookies)
	}
	protected := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	protected.AddCookie(cookies[0])
	protectedResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(protectedResponse, protected)
	if protectedResponse.Code != http.StatusOK {
		t.Fatalf("session did not authorize request: %d", protectedResponse.Code)
	}
}

func TestUnauthorizedAPIRequestRemainsJSON(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BasicAuth = &config.BasicAuthConfig{Username: "admin", Password: "secret"}
	s := NewServer(cfg, true)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status/content type = %d/%q", w.Code, w.Header().Get("Content-Type"))
	}
}

func TestMissingBrowserRouteUsesNotFoundUI(t *testing.T) {
	s := NewServer(config.DefaultConfig(), true)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status/content type = %d/%q", w.Code, w.Header().Get("Content-Type"))
	}
}
