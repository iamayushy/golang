package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/iamayushy/golang/internal/shortener"
)

const maxBodyBytes = 1 << 20

// Pinger reports whether a dependency (e.g. the database) is reachable.
type Pinger func(ctx context.Context) error

type Server struct {
	logger  *slog.Logger
	links   *shortener.Service
	baseURL string
	ping    Pinger
}

// New builds the HTTP handler with all routes and middleware.
func New(logger *slog.Logger, links *shortener.Service, baseURL string, ping Pinger) http.Handler {
	s := &Server{logger: logger, links: links, baseURL: baseURL, ping: ping}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /api/links", s.handleCreateLink)
	mux.HandleFunc("GET /api/links/{code}", s.handleGetLink)
	mux.HandleFunc("DELETE /api/links/{code}", s.handleDeleteLink)
	mux.HandleFunc("GET /{code}", s.handleRedirect)

	return recoverer(logger, requestLogger(logger, mux))
}

type linkResponse struct {
	shortener.Link
	ShortURL string `json:"short_url"`
}

func (s *Server) toResponse(link shortener.Link) linkResponse {
	return linkResponse{Link: link, ShortURL: s.baseURL + "/" + link.Code}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "url-shortener",
		"status":  "ok",
		"endpoints": []string{
			"POST /api/links",
			"GET /api/links/{code}",
			"DELETE /api/links/{code}",
			"GET /{code}",
			"GET /healthz",
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.ping(ctx); err != nil {
		s.logger.Error("health check failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	link, err := s.links.Shorten(r.Context(), req.URL)
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.toResponse(link))
}

func (s *Server) handleGetLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.links.Get(r.Context(), r.PathValue("code"))
	if err != nil {
		s.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toResponse(link))
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if err := s.links.Delete(r.Context(), r.PathValue("code")); err != nil {
		s.handleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	link, err := s.links.Resolve(r.Context(), r.PathValue("code"))
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
	case errors.Is(err, shortener.ErrInvalidURL):
		writeError(w, http.StatusBadRequest, err.Error())
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

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
		)
	})
}

func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered", "err", err, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
