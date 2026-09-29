package aisaas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_GetDevice_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/api/v1/devices/device123", r.URL.Path)
		assert.Equal(t, "device123", r.Header.Get("X-Device-Id"))
		assert.NotEmpty(t, r.Header.Get("X-Internal-Token"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deviceId":    "device123",
			"userId":      int64(1),
			"roleId":      int64(2),
			"tenantState": "active",
			"llmConfigId": 10,
			"ttsConfigId": 20,
			"sttConfigId": 30,
			"voiceName":   "shang",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	info, err := client.GetDevice(context.Background(), "device123")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "device123", info.DeviceID)
	assert.Equal(t, int64(1), info.UserID)
	assert.Equal(t, int64(2), info.RoleID)
	assert.Equal(t, "active", info.TenantState)
	assert.Equal(t, 10, info.LLMConfigID)
	assert.Equal(t, 20, info.TTSConfigID)
	assert.Equal(t, 30, info.STTConfigID)
	assert.Equal(t, "shang", info.VoiceName)
}

func TestClient_GetDevice_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("device not found"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetDevice(context.Background(), "device123")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=404")
}

func TestClient_GetDevice_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetDevice(context.Background(), "device123")

	require.Error(t, err)
}

func TestVerifyDeviceToken_NonEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deviceId": "device123",
			"userId":   int64(1),
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	err := client.VerifyDeviceToken(context.Background(), "device123", "non-empty-token")

	assert.NoError(t, err)
}

func TestVerifyDeviceToken_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deviceId": "device123",
			"userId":   int64(1),
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	err := client.VerifyDeviceToken(context.Background(), "device123", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty token")
}

func TestVerifyDeviceToken_PropagatesGetDeviceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	err := client.VerifyDeviceToken(context.Background(), "device123", "some-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=404")
}

func TestNewClient_SetsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-token", r.Header.Get("X-Internal-Token"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"deviceId": "x"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, _ = client.GetDevice(context.Background(), "x")
}
