package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamayushy/golang/internal/shortener"
)

const baseURL = "http://sho.rt"

func newTestHandler(opts Options) http.Handler {
	if opts.Ping == nil {
		opts.Ping = func(context.Context) error { return nil }
	}
	opts.BaseURL = baseURL
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := shortener.NewService(shortener.NewMemoryRepository())
	return New(logger, svc, opts)
}

func do(h http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) linkResponse {
	t.Helper()
	var resp linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return resp
}

func TestIndex(t *testing.T) {
	h := newTestHandler(Options{})
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if rec := do(h, method, "/", ""); rec.Code != http.StatusOK {
			t.Fatalf("%s / status = %d, want 200", method, rec.Code)
		}
	}

	h = newTestHandler(Options{FrontendURL: "https://app.example"})
	rec := do(h, http.MethodGet, "/", "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.example" {
		t.Fatalf("/ with frontend = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestHealth(t *testing.T) {
	if rec := do(newTestHandler(Options{}), http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	down := newTestHandler(Options{Ping: func(context.Context) error { return errors.New("db down") }})
	if rec := do(down, http.MethodGet, "/healthz", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestLinkLifecycle(t *testing.T) {
	h := newTestHandler(Options{})

	rec := do(h, http.MethodPost, "/api/links", `{"url":"https://example.com/docs","ttl_minutes":25}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", rec.Code, rec.Body)
	}
	created := decode(t, rec)
	if created.ShortURL != baseURL+"/"+created.Code {
		t.Fatalf("short_url = %q", created.ShortURL)
	}
	if created.DeleteToken == "" {
		t.Fatal("create response has no delete_token")
	}
	if s := created.ExpiresInSeconds; s < 24*60 || s > 25*60 {
		t.Fatalf("expires_in_seconds = %d, want about 1500", s)
	}

	rec = do(h, http.MethodGet, "/"+created.Code, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://example.com/docs" {
		t.Fatalf("redirect = %d to %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = do(h, http.MethodGet, "/api/links/"+created.Code, "")
	stats := decode(t, rec)
	if rec.Code != http.StatusOK || stats.Clicks != 1 || stats.DeleteToken != "" {
		t.Fatalf("stats = %d %+v, want 200 with 1 click and no token", rec.Code, stats)
	}

	if rec = do(h, http.MethodDelete, "/api/links/"+created.Code, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("delete without token status = %d, want 403", rec.Code)
	}
	rec = do(h, http.MethodDelete, "/api/links/"+created.Code, "", "X-Delete-Token", created.DeleteToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec = do(h, http.MethodGet, "/"+created.Code, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("redirect after delete = %d, want 404", rec.Code)
	}
}

func TestDeadLinkRedirectsToFrontend(t *testing.T) {
	h := newTestHandler(Options{FrontendURL: "https://app.example"})
	rec := do(h, http.MethodGet, "/nope42", "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://app.example/?gone=nope42" {
		t.Fatalf("dead link = %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestCreateLinkBadInput(t *testing.T) {
	h := newTestHandler(Options{})
	for _, body := range []string{`not json`, `{"url":"not-a-url"}`, `{}`, `{"url":"https://a.b","ttl_minutes":60}`} {
		if rec := do(h, http.MethodPost, "/api/links", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestCORS(t *testing.T) {
	h := newTestHandler(Options{AllowedOrigins: []string{"https://app.example"}})

	rec := do(h, http.MethodOptions, "/api/links", "",
		"Origin", "https://app.example", "Access-Control-Request-Method", "POST")
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("preflight = %d, allow-origin %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}

	rec = do(h, http.MethodGet, "/healthz", "", "Origin", "https://evil.example")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin got allow-origin %q", got)
	}
}

func TestCreateRateLimit(t *testing.T) {
	h := newTestHandler(Options{CreatesPerMinute: 2})
	body := `{"url":"https://example.com"}`
	for i := range 2 {
		if rec := do(h, http.MethodPost, "/api/links", body); rec.Code != http.StatusCreated {
			t.Fatalf("create %d status = %d", i, rec.Code)
		}
	}
	if rec := do(h, http.MethodPost, "/api/links", body); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third create status = %d, want 429", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/api/links", body, "X-Forwarded-For", "203.0.113.9"); rec.Code != http.StatusCreated {
		t.Fatalf("create from another IP status = %d, want 201", rec.Code)
	}
}
