package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
)

func TestHealthz(t *testing.T) {
	cfg := &config.Config{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	r := SetupRouter(cfg)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	cfg := &config.Config{}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	r := SetupRouter(cfg)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAuthMetrics_RejectsMissingToken(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			InternalToken: "test-secret-token",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	r := SetupRouter(cfg)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMetrics_RejectsWrongToken(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			InternalToken: "test-secret-token",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	r := SetupRouter(cfg)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMetrics_AcceptsCorrectToken(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			InternalToken: "test-secret-token",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	rec := httptest.NewRecorder()

	r := SetupRouter(cfg)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
