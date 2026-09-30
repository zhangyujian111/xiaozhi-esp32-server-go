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
	assert.Equal(t, int64(1), info.TenantID)
	assert.Equal(t, int64(1), info.UserID)
	assert.Equal(t, "abc123", info.BindCode)
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

func TestClient_GetPersonaByDevice_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/api/v1/personas/by-device", r.URL.Path)
		assert.Equal(t, "device123", r.URL.Query().Get("deviceId"))
		assert.Equal(t, "1001", r.URL.Query().Get("tenantId"))
		assert.Equal(t, "test-token", r.Header.Get("X-Internal-Token"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"code": 0,
			"data": [{
				"id": "2105202601480949760",
				"code": "hakumi02",
				"name": "hakumi",
				"modelType": "llm",
				"tenantId": "1001",
				"systemPrompt": "You are Hakumi.",
				"defaultModelId": "demo-chat",
				"temperature": 0.7,
				"topP": 0.9,
				"maxTokens": 4096,
				"memoryType": "none"
			}]
		}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	p, err := client.GetPersonaByDevice(context.Background(), "device123", 1001)

	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, int64(2105202601480949760), p.ID)
	assert.Equal(t, "hakumi02", p.Code)
	assert.Equal(t, "hakumi", p.Name)
	assert.Equal(t, "You are Hakumi.", p.SystemPrompt)
	assert.Equal(t, int64(1001), p.TenantID)
}

func TestClient_GetPersonaByDevice_NotBound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":40404,"message":"设备未绑定 Persona"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetPersonaByDevice(context.Background(), "device123", 1001)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPersonaNotBound)
}

func TestClient_GetPersonaByDevice_EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"data":[]}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetPersonaByDevice(context.Background(), "device123", 1001)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPersonaNotBound)
}

func TestClient_GetPersonaByDevice_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	_, err := client.GetPersonaByDevice(context.Background(), "device123", 1001)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status=500")
}
