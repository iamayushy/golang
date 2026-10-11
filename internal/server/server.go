package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iamayushy/golang/internal/shortener"
)

const maxBodyBytes = 1 << 20

// Pinger reports whether a dependency (e.g. the database) is reachable.
type Pinger func(ctx context.Context) error

type Options struct {
	// BaseURL is the public origin of this API, used to build short URLs.
	// If empty it is worked out from each request (see publicBase).
	BaseURL string
	// FrontendURL, if set, is where / and dead short links redirect to.
	FrontendURL string
	// AllowedOrigins lists origins allowed by CORS; "*" allows any.
	AllowedOrigins []string
	// CreatesPerMinute caps link creation per client IP; 0 disables the limit.
	CreatesPerMinute int
	Ping             Pinger
}

type Server struct {
	logger *slog.Logger
	links  *shortener.Service
	opts   Options
}

// New builds the HTTP handler with all routes and middleware.
func New(logger *slog.Logger, links *shortener.Service, opts Options) http.Handler {
	s := &Server{logger: logger, links: links, opts: opts}

	create := http.Handler(http.HandlerFunc(s.handleCreateLink))
	if opts.CreatesPerMinute > 0 {
		create = rateLimit(newRateLimiter(opts.CreatesPerMinute, time.Minute), create)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("POST /api/links", create)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/links/{code}", s.handleGetLink)
	mux.HandleFunc("DELETE /api/links/{code}", s.handleDeleteLink)
	mux.HandleFunc("GET /{code}", s.handleRedirect)

	return recoverer(logger, requestLogger(logger, cors(opts.AllowedOrigins, mux)))
}

type linkResponse struct {
	shortener.Link
	ShortURL         string `json:"short_url"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
	DeleteToken      string `json:"delete_token,omitempty"`
}

func (s *Server) toResponse(r *http.Request, link shortener.Link) linkResponse {
	return linkResponse{
		Link:             link,
		ShortURL:         s.publicBase(r) + "/" + link.Code,
		ExpiresInSeconds: int64(s.links.TimeLeft(link).Seconds()),
	}
}

// publicBase is the origin clients reached us on. Behind a TLS-terminating proxy
// the request arrives as plain http, so the proxy's forwarded headers win.
func (s *Server) publicBase(r *http.Request) string {
	if s.opts.BaseURL != "" {
		return s.opts.BaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := firstHeaderValue(r, "X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if fwd := firstHeaderValue(r, "X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host
}

func firstHeaderValue(r *http.Request, name string) string {
	v, _, _ := strings.Cut(r.Header.Get(name), ",")
	return strings.TrimSpace(v)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.opts.FrontendURL != "" {
		http.Redirect(w, r, s.opts.FrontendURL, http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "url-shortener",
		"status":  "ok",
		"endpoints": []string{
			"POST /api/links",
			"GET /api/links/{code}",
			"GET /api/stats",
			"DELETE /api/links/{code}",
			"GET /{code}",
			"GET /healthz",
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.opts.Ping(ctx); err != nil {
		s.logger.Error("health check failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL        string `json:"url"`
		TTLMinutes int    `json:"ttl_minutes"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	created, err := s.links.Shorten(r.Context(), req.URL, time.Duration(req.TTLMinutes)*time.Minute)
	if err != nil {
		s.handleError(w, err)
		return
	}
	resp := s.toResponse(r, created.Link)
	resp.DeleteToken = created.DeleteToken
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.links.Stats(r.Context())
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleGetLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.links.Get(r.Context(), r.PathValue("code"))
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toResponse(r, link))
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	err := s.links.Delete(r.Context(), r.PathValue("code"), r.Header.Get("X-Delete-Token"))
	if err != nil {
		s.handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	link, err := s.links.Resolve(r.Context(), code)
	if errors.Is(err, shortener.ErrNotFound) && s.opts.FrontendURL != "" {
		http.Redirect(w, r, s.opts.FrontendURL+"/?gone="+url.QueryEscape(code), http.StatusFound)
		return
	}
	if err != nil {
		s.handleError(w, err)
		return
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (s *Server) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, shortener.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, shortener.ErrInvalidURL), errors.Is(err, shortener.ErrInvalidTTL):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, shortener.ErrInvalidToken):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		s.logger.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
