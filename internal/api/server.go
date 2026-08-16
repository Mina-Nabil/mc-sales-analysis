// Package api serves the REST/JSON API and the embedded SPA (TECH §7, §1).
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mina-Nabil/mc-sales-analysis/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey int

const userKey ctxKey = 0

// Server holds the API dependencies.
type Server struct {
	pool      *pgxpool.Pool
	uploadDir string
	web       fs.FS
	files     http.Handler
	secure    bool // set Secure flag on cookies (false for plain-HTTP local dev)
}

// New builds a Server. spaFS is the embedded web build rooted at "web/dist".
func New(pool *pgxpool.Pool, uploadDir string, spaFS fs.FS, secure bool) (*Server, error) {
	sub, err := fs.Sub(spaFS, "web/dist")
	if err != nil {
		return nil, err
	}
	return &Server{
		pool:      pool,
		uploadDir: uploadDir,
		web:       sub,
		files:     http.FileServer(http.FS(sub)),
		secure:    secure,
	}, nil
}

// spaHandler serves embedded static assets, falling back to index.html for
// client-side routes (deep links like /review) so the SPA router can handle them.
func (s *Server) spaHandler(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p != "" {
		if f, err := s.web.Open(p); err == nil {
			f.Close()
			s.files.ServeHTTP(w, r)
			return
		}
	}
	index, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		http.Error(w, "front-end not built", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(index)
}

// Handler returns the fully-wired HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// auth
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/v1/auth/me", s.auth(s.me))
	mux.HandleFunc("POST /api/v1/users", s.auth(s.createUser))

	// review queue
	mux.HandleFunc("GET /api/v1/review", s.auth(s.reviewList))
	mux.HandleFunc("POST /api/v1/review/bulk-confirm", s.auth(s.reviewBulkConfirm))
	mux.HandleFunc("POST /api/v1/review/{id}/confirm", s.auth(s.reviewConfirm))
	mux.HandleFunc("POST /api/v1/review/{id}/reject", s.auth(s.reviewReject))
	mux.HandleFunc("POST /api/v1/review/{id}/reassign", s.auth(s.reviewReassign))
	mux.HandleFunc("POST /api/v1/review/{id}/new-model", s.auth(s.reviewNewModel))
	mux.HandleFunc("POST /api/v1/resolve", s.auth(s.resolve))
	mux.HandleFunc("GET /api/v1/review/brands", s.auth(s.brandQueue))
	mux.HandleFunc("POST /api/v1/review/brands/resolve", s.auth(s.resolveBrand))

	// car tree (read)
	mux.HandleFunc("GET /api/v1/brands", s.auth(s.brands))
	mux.HandleFunc("GET /api/v1/brands/{id}/models", s.auth(s.brandModels))
	mux.HandleFunc("GET /api/v1/models/{id}/aliases", s.auth(s.modelAliases))
	mux.HandleFunc("GET /api/v1/segments", s.auth(s.segments))
	mux.HandleFunc("GET /api/v1/distributors", s.auth(s.distributors))

	// car tree (mutations)
	mux.HandleFunc("POST /api/v1/brands", s.auth(s.createBrand))
	mux.HandleFunc("POST /api/v1/models", s.auth(s.createModel))
	mux.HandleFunc("PATCH /api/v1/models/{id}", s.auth(s.editModel))
	mux.HandleFunc("GET /api/v1/models/{id}/merge-preview", s.auth(s.mergePreview))
	mux.HandleFunc("POST /api/v1/models/{id}/merge", s.auth(s.mergeModels))
	mux.HandleFunc("DELETE /api/v1/aliases/{id}", s.auth(s.deleteAlias))

	// imports
	mux.HandleFunc("POST /api/v1/imports", s.auth(s.importUpload))
	mux.HandleFunc("GET /api/v1/imports", s.auth(s.importList))
	mux.HandleFunc("GET /api/v1/imports/{id}/dry-run", s.auth(s.importDryRun))
	mux.HandleFunc("POST /api/v1/imports/{id}/commit", s.auth(s.importCommit))

	// misc
	mux.HandleFunc("GET /api/v1/settings", s.auth(s.getSettings))
	mux.HandleFunc("PATCH /api/v1/settings", s.auth(s.patchSettings))
	mux.HandleFunc("GET /api/v1/changes", s.auth(s.changes))
	mux.HandleFunc("GET /api/v1/analytics/confidence", s.auth(s.confidence))
	mux.HandleFunc("GET /api/v1/analytics/matrix", s.auth(s.analyticsMatrix))
	mux.HandleFunc("GET /api/v1/analytics/dimensions", s.auth(s.analyticsDimensions))
	mux.HandleFunc("GET /api/v1/analytics/years", s.auth(s.analyticsYears))
	mux.HandleFunc("GET /api/v1/analytics/export.xlsx", s.auth(s.analyticsExport))
	mux.HandleFunc("GET /api/v1/stats", s.auth(s.stats))

	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /", s.spaHandler) // SPA + static assets (with deep-link fallback)

	return logRequests(mux)
}

// auth wraps a handler, requiring a valid session cookie and injecting the user.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.CookieName)
		if err != nil {
			httpErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		u, err := auth.Authenticate(r.Context(), s.pool, c.Value)
		if err != nil {
			httpErr(w, http.StatusUnauthorized, "session expired or invalid")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, u)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) user(r *http.Request) auth.User {
	u, _ := r.Context().Value(userKey).(auth.User)
	return u
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		httpErr(w, http.StatusServiceUnavailable, "db unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func pathInt(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

func parseIntQ(s string) (int, error)       { return strconv.Atoi(s) }
func parseFloatQ(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if r.URL.Path != "/healthz" {
			logLine(r.Method, r.URL.Path, sw.status, time.Since(start))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func logLine(method, path string, status int, dur time.Duration) {
	slog.Info("http", "method", method, "path", path, "status", status, "ms", dur.Milliseconds())
}
