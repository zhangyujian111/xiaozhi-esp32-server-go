package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
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

type mockStore struct{}

func (m *mockStore) GetDevice(ctx context.Context, deviceID string) (*store.Device, error) {
	return nil, store.ErrDeviceNotFound
}
func (m *mockStore) UpsertDevice(ctx context.Context, device *store.Device) error {
	return nil
}
func (m *mockStore) ListDevices(ctx context.Context, limit, offset int) ([]store.Device, error) {
	return nil, nil
}
func (m *mockStore) ActivateDevice(ctx context.Context, deviceID, clientID, serialNumber string) error {
	return nil
}
func (m *mockStore) UpdateDevice(ctx context.Context, device *store.Device) error {
	return nil
}
func (m *mockStore) DeleteDevice(ctx context.Context, deviceID string) error {
	return nil
}
func (m *mockStore) BindDevice(ctx context.Context, deviceID string, userID int64) error {
	return nil
}
func (m *mockStore) UnbindDevice(ctx context.Context, deviceID string) error {
	return nil
}

func TestSetupAPIRouter_OTAPath_400WithoutHeaders(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			InternalToken: "test-secret",
		},
		Aisaas: config.AisaasConfig{
			BaseURL:       "http://localhost:9999",
			InternalToken: "aisaas-token",
		},
	}
	r := SetupAPIRouter(cfg, &mockStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/device/ota", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
