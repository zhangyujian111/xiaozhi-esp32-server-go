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
		assert.Equal(t, "/internal/api/v1/devices/device123/bind-code", r.URL.Path)
		assert.Equal(t, "device123", r.Header.Get("X-Device-Id"))
		assert.NotEmpty(t, r.Header.Get("X-Internal-Token"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"deviceId": "device123",
			"tenantId": int64(1),
			"bindCode": "abc123",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	info, err := client.GetDevice(context.Background(), "device123")

	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "device123", info.DeviceID)
	assert.Equal(t, int64(1), info.UserID)
}

func TestClient_GetDevice_NotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":40404,"message":"device not registered"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetDevice(context.Background(), "device123")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeviceNotRegistered)
}

func TestClient_GetDevice_OtherError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetDevice(context.Background(), "device123")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=500")
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
	assert.ErrorIs(t, err, ErrDeviceNotRegistered)
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
