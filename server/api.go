package main

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	sessionCookie  = "tk_session"
	sessionTTL     = 12 * time.Hour
	maxBodyBytes   = 512 << 10
	maxProfileSize = 256 << 10
	maxLinks       = 20
	pbkdf2Iter     = 600_000
)

type Server struct {
	store   *Store
	limiter *loginLimiter
}

func (s *Server) routes(static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.store.adminCount(); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.auth(s.handleMe))
	mux.HandleFunc("GET /api/packages", s.auth(s.handleList))
	mux.HandleFunc("POST /api/packages", s.auth(s.handleCreate))
	mux.HandleFunc("GET /api/packages/{id}", s.auth(s.handleGet))
	mux.HandleFunc("PUT /api/packages/{id}", s.auth(s.handleUpdate))
	mux.HandleFunc("DELETE /api/packages/{id}", s.auth(s.handleDelete))
	mux.HandleFunc("GET /api/packages/{id}/codes", s.auth(s.handleCodes))
	mux.HandleFunc("POST /api/totp/preview", s.auth(s.handleTOTPPreview))
	mux.Handle("/", static)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			// Cross-site request guard: browsers can't add custom headers to
			// cross-origin requests without a CORS preflight, which we never allow.
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Tunnelkey") != "1" {
				writeError(w, http.StatusForbidden, "missing X-Tunnelkey header")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Auth ----------------------------------------------------------------

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

func checkPassword(encoded, password string) bool {
	f := strings.Split(encoded, "$")
	if len(f) != 4 || f[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err1 := strconv.Atoi(f[1])
	salt, err2 := hex.DecodeString(f[2])
	want, err3 := hex.DecodeString(f[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

// A throwaway hash so unknown usernames cost as much time as known ones.
var dummyHash, _ = hashPassword("tunnelkey-dummy")

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-15 * time.Minute)
	recent := l.failures[ip][:0]
	for _, t := range l.failures[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	l.failures[ip] = recent
	return len(recent) >= 10
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	l.failures[ip] = append(l.failures[ip], time.Now())
	l.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.limiter.blocked(ip) {
		writeError(w, http.StatusTooManyRequests, "Too many failed attempts. Try again in 15 minutes.")
		return
	}
	var req struct{ Username, Password string }
	if !readJSON(w, r, &req) {
		return
	}
	id, hash, err := s.store.adminByName(strings.TrimSpace(req.Username))
	if err != nil {
		checkPassword(dummyHash, req.Password)
		s.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	if !checkPassword(hash, req.Password) {
		s.limiter.fail(ip)
		writeError(w, http.StatusUnauthorized, "Wrong username or password.")
		return
	}
	token, err := s.store.createSession(id, sessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	writeJSON(w, map[string]string{"username": req.Username})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.store.deleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

type ctxHandler func(w http.ResponseWriter, r *http.Request, admin string)

func (s *Server) auth(h ctxHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not signed in")
			return
		}
		admin, err := s.store.sessionAdmin(c.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		h(w, r, admin)
	}
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, admin string) {
	writeJSON(w, map[string]string{"username": admin})
}

// ---- Packages ------------------------------------------------------------

type packageRequest struct {
	Name         string      `json:"name"`
	OVPN         string      `json:"ovpn"` // empty on update = keep
	Username     string      `json:"username"`
	Password     string      `json:"password"`
	KeepPassword bool        `json:"keepPassword"`
	TOTP         *TOTPConfig `json:"totp"`
	KeepTOTP     bool        `json:"keepTotp"`
	ManualCode   bool        `json:"manualCode"`
	CodePosition string      `json:"codePosition"`
	Links        []Link      `json:"links"`
}

// packageView is a package without its secrets, for the edit form.
type packageView struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	OVPN         string    `json:"ovpn"`
	Username     string    `json:"username"`
	HasPassword  bool      `json:"hasPassword"`
	TOTP         *totpView `json:"totp"`
	ManualCode   bool      `json:"manualCode"`
	CodePosition string    `json:"codePosition"`
	Links        []Link    `json:"links"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type totpView struct {
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Algorithm string `json:"algorithm"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request, _ string) {
	list, err := s.store.listPackages()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request, _ string) {
	p, ok := s.loadPackage(w, r)
	if !ok {
		return
	}
	v := packageView{
		ID: p.ID, Name: p.Name, OVPN: p.OVPN, Username: p.Username, HasPassword: p.Password != "",
		ManualCode: p.ManualCode, CodePosition: p.CodePosition, Links: p.Links, UpdatedAt: p.UpdatedAt,
	}
	if p.TOTP != nil {
		v.TOTP = &totpView{Digits: p.TOTP.Digits, Period: p.TOTP.Period, Algorithm: p.TOTP.Algorithm}
	}
	writeJSON(w, v)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request, admin string) {
	var req packageRequest
	if !readJSON(w, r, &req) {
		return
	}
	id, _ := randomID(12)
	now := time.Now().UTC()
	p := &Package{ID: strings.ToLower(id), CreatedAt: now}
	if err := applyRequest(p, &req, nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.UpdatedAt = now
	if err := s.store.savePackage(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("admin %q created package %s (%q)", admin, p.ID, p.Name)
	writeJSON(w, summarize(p))
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request, admin string) {
	existing, ok := s.loadPackage(w, r)
	if !ok {
		return
	}
	var req packageRequest
	if !readJSON(w, r, &req) {
		return
	}
	p := &Package{ID: existing.ID, CreatedAt: existing.CreatedAt}
	if err := applyRequest(p, &req, existing); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.UpdatedAt = time.Now().UTC()
	if err := s.store.savePackage(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("admin %q updated package %s", admin, p.ID)
	writeJSON(w, summarize(p))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, admin string) {
	if err := s.store.deletePackage(r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("admin %q deleted package %s", admin, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadPackage(w http.ResponseWriter, r *http.Request) (*Package, bool) {
	p, err := s.store.getPackage(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "package not found")
		return nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return p, true
}

// applyRequest validates req and copies it into p. existing is nil on create.
func applyRequest(p *Package, req *packageRequest, existing *Package) error {
	p.Name = strings.TrimSpace(req.Name)
	if p.Name == "" || len([]rune(p.Name)) > 80 {
		return errors.New("Name is required (up to 80 characters).")
	}

	p.OVPN = normalizeNewlines(req.OVPN)
	if p.OVPN == "" && existing != nil {
		p.OVPN = existing.OVPN
	}
	if strings.TrimSpace(p.OVPN) == "" {
		return errors.New("An .ovpn profile is required.")
	}
	if len(p.OVPN) > maxProfileSize {
		return errors.New("The .ovpn profile is too large.")
	}
	if firstRemote(p.OVPN) == "" {
		return errors.New("The .ovpn profile has no “remote” line.")
	}
	for _, d := range []string{"ca", "cert", "key", "tls-auth", "tls-crypt", "pkcs12"} {
		for _, line := range strings.Split(p.OVPN, "\n") {
			f := strings.Fields(line)
			if len(f) >= 2 && f[0] == d {
				return fmt.Errorf("The profile references the file %q. Embed it inline (<%s>…</%s>) — the phone can't read separate files.", f[1], d, d)
			}
		}
	}

	p.Username = strings.TrimSpace(req.Username)
	p.Password = req.Password
	if p.Password == "" && req.KeepPassword && existing != nil {
		p.Password = existing.Password
	}

	switch {
	case req.TOTP != nil && strings.TrimSpace(req.TOTP.Secret) != "":
		t := *req.TOTP
		if err := t.normalize(); err != nil {
			return err
		}
		p.TOTP = &t
	case req.KeepTOTP && existing != nil && existing.TOTP != nil:
		t := *existing.TOTP
		if req.TOTP != nil { // allow changing digits/period/algorithm without re-entering the secret
			t.Digits, t.Period, t.Algorithm = req.TOTP.Digits, req.TOTP.Period, req.TOTP.Algorithm
		}
		if err := t.normalize(); err != nil {
			return err
		}
		p.TOTP = &t
	}
	p.ManualCode = req.ManualCode && p.TOTP == nil
	p.CodePosition = "after"
	if req.CodePosition == "before" {
		p.CodePosition = "before"
	}

	if len(req.Links) > maxLinks {
		return fmt.Errorf("At most %d links.", maxLinks)
	}
	p.Links = nil
	for i, l := range req.Links {
		l.Title = strings.TrimSpace(l.Title)
		if l.Title == "" || len([]rune(l.Title)) > 60 {
			return fmt.Errorf("Link %d needs a title (up to 60 characters).", i+1)
		}
		switch l.Kind {
		case "web":
			u, err := url.Parse(strings.TrimSpace(l.URL))
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
				return fmt.Errorf("Link “%s”: enter a full http(s):// address.", l.Title)
			}
			l = Link{Title: l.Title, Kind: "web", URL: u.String()}
		case "rdp":
			host := strings.TrimSpace(l.Host)
			if host == "" || strings.ContainsAny(host, " /&?") {
				return fmt.Errorf("Link “%s”: enter the computer's host name or IP address.", l.Title)
			}
			if l.Port == 0 {
				l.Port = 3389
			}
			if l.Port < 1 || l.Port > 65535 {
				return fmt.Errorf("Link “%s”: port must be 1–65535.", l.Title)
			}
			l = Link{Title: l.Title, Kind: "rdp", Host: host, Port: l.Port, Username: strings.TrimSpace(l.Username)}
		case "app":
			u, err := url.Parse(strings.TrimSpace(l.URL))
			if err != nil || u.Scheme == "" {
				return fmt.Errorf("Link “%s”: enter a URI with a scheme, e.g. myapp://open.", l.Title)
			}
			switch strings.ToLower(u.Scheme) {
			case "javascript", "data", "file", "vbscript":
				return fmt.Errorf("Link “%s”: %s: links are not allowed.", l.Title, u.Scheme)
			}
			l = Link{Title: l.Title, Kind: "app", URL: u.String()}
		default:
			return fmt.Errorf("Link “%s”: unknown kind %q.", l.Title, l.Kind)
		}
		p.Links = append(p.Links, l)
	}
	return nil
}

// ---- Setup codes ---------------------------------------------------------

type codeImage struct {
	Index int    `json:"index"`
	Total int    `json:"total"`
	Image string `json:"image"` // data: URL (PNG)
	Chars int    `json:"chars"`
}

func (s *Server) handleCodes(w http.ResponseWriter, r *http.Request, admin string) {
	p, ok := s.loadPackage(w, r)
	if !ok {
		return
	}
	texts, err := encodeSetupCodes(buildPayload(p))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	level := qrcode.Low
	if len(texts) > 1 {
		level = qrcode.Medium
	}
	out := make([]codeImage, 0, len(texts))
	for i, t := range texts {
		q, err := qrcode.New(t, level)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "QR generation failed: "+err.Error())
			return
		}
		png, err := q.PNG(-6) // 6 px per module
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, codeImage{
			Index: i + 1, Total: len(texts), Chars: len(t),
			Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		})
	}
	log.Printf("admin %q displayed setup codes for package %s", admin, p.ID)
	writeJSON(w, out)
}

func (s *Server) handleTOTPPreview(w http.ResponseWriter, r *http.Request, _ string) {
	var req struct {
		URI       string `json:"uri"`
		Secret    string `json:"secret"`
		Digits    int    `json:"digits"`
		Period    int    `json:"period"`
		Algorithm string `json:"algorithm"`
		PackageID string `json:"packageId"` // preview the stored secret
	}
	if !readJSON(w, r, &req) {
		return
	}
	var t TOTPConfig
	var err error
	switch {
	case req.URI != "":
		t, err = parseOTPAuthURI(req.URI)
	case req.Secret == "" && req.PackageID != "":
		p, e := s.store.getPackage(req.PackageID)
		if e != nil || p.TOTP == nil {
			writeError(w, http.StatusBadRequest, "no stored secret")
			return
		}
		t = *p.TOTP
		t.Digits, t.Period, t.Algorithm = req.Digits, req.Period, req.Algorithm
		err = t.normalize()
	default:
		t = TOTPConfig{Secret: req.Secret, Digits: req.Digits, Period: req.Period, Algorithm: req.Algorithm}
		err = t.normalize()
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	code, _ := t.code(now)
	resp := map[string]any{
		"code":        code,
		"secondsLeft": t.Period - int(now.Unix()%int64(t.Period)),
		"digits":      t.Digits,
		"period":      t.Period,
		"algorithm":   t.Algorithm,
	}
	if req.URI != "" {
		resp["secret"] = t.Secret // so the form can hold it after parsing the URI
	}
	writeJSON(w, resp)
}

// ---- helpers -------------------------------------------------------------

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
