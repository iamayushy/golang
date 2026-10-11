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

func newTestHandler(ping Pinger) http.Handler {
	if ping == nil {
		ping = func(context.Context) error { return nil }
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := shortener.NewService(shortener.NewMemoryRepository())
	return New(logger, svc, baseURL, ping)
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIndex(t *testing.T) {
	h := newTestHandler(nil)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		if rec := do(h, method, "/", ""); rec.Code != http.StatusOK {
			t.Fatalf("%s / status = %d, want 200", method, rec.Code)
		}
	}
}

func TestHealth(t *testing.T) {
	if rec := do(newTestHandler(nil), http.MethodGet, "/healthz", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	down := newTestHandler(func(context.Context) error { return errors.New("db down") })
	if rec := do(down, http.MethodGet, "/healthz", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestLinkLifecycle(t *testing.T) {
	h := newTestHandler(nil)

	rec := do(h, http.MethodPost, "/api/links", `{"url":"https://example.com/docs"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", rec.Code, rec.Body)
	}
	var created linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ShortURL != baseURL+"/"+created.Code {
		t.Fatalf("short_url = %q", created.ShortURL)
	}

	rec = do(h, http.MethodGet, "/"+created.Code, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://example.com/docs" {
		t.Fatalf("redirect = %d to %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = do(h, http.MethodGet, "/api/links/"+created.Code, "")
	var stats linkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || stats.Clicks != 1 {
		t.Fatalf("stats = %d %+v, want 200 with 1 click", rec.Code, stats)
	}

	if rec = do(h, http.MethodDelete, "/api/links/"+created.Code, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec = do(h, http.MethodGet, "/"+created.Code, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("redirect after delete = %d, want 404", rec.Code)
	}
}

func TestCreateLinkBadInput(t *testing.T) {
	h := newTestHandler(nil)
	for _, body := range []string{`not json`, `{"url":"not-a-url"}`, `{}`} {
		if rec := do(h, http.MethodPost, "/api/links", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}
