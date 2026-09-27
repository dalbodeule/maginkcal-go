package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	sessionCookieName = "epdcal_session"
	sessionLifetime   = 12 * time.Hour
)

type session struct {
	expiresAt time.Time
}

func (s *Server) basicAuthConfigured() bool {
	cfg := s.Config()
	return cfg != nil && cfg.BasicAuth != nil && cfg.BasicAuth.Username != "" && cfg.BasicAuth.Password != ""
}

func isPublicAuthPath(path string) bool {
	return path == "/health" || path == "/login" || path == "/logout" ||
		path == "/401" || path == "/401/" || path == "/404" ||
		path == "/404/" || path == "/app-config.js" || path == "/favicon.png" ||
		strings.HasPrefix(path, "/_next/")
}

func (s *Server) hasSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	entry, ok := s.sessions[cookie.Value]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.sessions, cookie.Value)
		return false
	}
	return true
}

func (s *Server) createSession() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	s.sessionsMu.Lock()
	s.sessions[id] = session{expiresAt: time.Now().Add(sessionLifetime)}
	s.sessionsMu.Unlock()
	return id, nil
}

func (s *Server) clearSessions() {
	s.sessionsMu.Lock()
	s.sessions = make(map[string]session)
	s.sessionsMu.Unlock()
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.basicAuthConfigured() {
		http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	if r.Method == http.MethodGet {
		s.serveEmbeddedLoginPage(w, http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
		return
	}
	cfg := s.Config()
	if !secureCompare(r.FormValue("username"), cfg.BasicAuth.Username) ||
		!secureCompare(r.FormValue("password"), cfg.BasicAuth.Password) {
		http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
		return
	}
	id, err := s.createSession()
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionLifetime.Seconds()),
	})
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessionsMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login?logged_out=1", http.StatusSeeOther)
}

func safeNext(value string) string {
	if value == "" {
		return "/"
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil || !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") {
		return "/"
	}
	return decoded
}
