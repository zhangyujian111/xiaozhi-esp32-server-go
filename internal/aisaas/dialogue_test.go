package aisaas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
)

func TestClient_Dialogue_HappyPath(t *testing.T) {
	var capturedReq struct {
		DeviceID    string `json:"deviceId"`
		AudioFormat string `json:"audioFormat"`
		SampleRate  int    `json:"sampleRate"`
		AudioBase64 string `json:"audioBase64"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/api/v1/dialogue/run", r.URL.Path)
		assert.Equal(t, "test-token", r.Header.Get("X-Internal-Token"))

		var req struct {
			DeviceID    string `json:"deviceId"`
			AudioFormat string `json:"audioFormat"`
			SampleRate  int    `json:"sampleRate"`
			AudioBase64 string `json:"audioBase64"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		capturedReq = req

		// Verify it's valid base64 wav
		_, err = base64.StdEncoding.DecodeString(req.AudioBase64)
		require.NoError(t, err, "audioBase64 must be valid base64")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"userText":    "今天天气怎么样",
				"replyText":   "今天北京天气晴，气温20度。",
				"audioFormat": "opus",
				"sampleRate":  24000,
				"audioBase64": base64.StdEncoding.EncodeToString([]byte("fake-opus-data")),
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	pcmData := make([]byte, 16000) // 1 second of silence at 16kHz
	wavData := wav.PCMToWAV(pcmData, 16000, 1, 16)

	result, err := client.Dialogue(context.Background(), "28:84:85:4b:43:a4", wavData)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "今天天气怎么样", result.UserText)
	assert.Equal(t, "今天北京天气晴，气温20度。", result.ReplyText)
	assert.Equal(t, "opus", result.AudioFormat)
	assert.Equal(t, 24000, result.SampleRate)
	assert.Equal(t, []byte("fake-opus-data"), result.Audio)

	// Verify request fields
	assert.Equal(t, "28:84:85:4b:43:a4", capturedReq.DeviceID)
	assert.Equal(t, "wav", capturedReq.AudioFormat)
	assert.Equal(t, 16000, capturedReq.SampleRate)
}

func TestClient_Dialogue_DeviceNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    40404,
			"message": "device not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	pcmData := make([]byte, 16000)
	wavData := wav.PCMToWAV(pcmData, 16000, 1, 16)

	result, err := client.Dialogue(context.Background(), "unknown-device", wavData)

	require.Error(t, err)
	assert.Nil(t, result)
	// ErrDeviceNotRegistered message is "device not registered with aisaas"
	assert.ErrorIs(t, err, ErrDeviceNotRegistered)
}

func TestClient_Dialogue_QuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    42901,
			"message": "quota exceeded",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	pcmData := make([]byte, 16000)
	wavData := wav.PCMToWAV(pcmData, 16000, 1, 16)

	result, err := client.Dialogue(context.Background(), "28:84:85:4b:43:a4", wavData)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "quota exceeded")
}

func TestClient_Dialogue_ModelNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    40401,
			"message": "stt model not found",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	pcmData := make([]byte, 16000)
	wavData := wav.PCMToWAV(pcmData, 16000, 1, 16)

	result, err := client.Dialogue(context.Background(), "28:84:85:4b:43:a4", wavData)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "model not found")
}

func TestClient_Dialogue_BadBase64(t *testing.T) {
	// Empty wav bytes are valid base64 (empty string) and should be sent to the server.
	// The server handles the empty audio case; the client just encodes it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AudioBase64 string `json:"audioBase64"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "", req.AudioBase64, "empty audio encodes to empty base64")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"userText":    "",
				"replyText":   "",
				"audioFormat": "opus",
				"sampleRate":  24000,
				"audioBase64": "",
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	result, err := client.Dialogue(context.Background(), "device1", []byte{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "", result.UserText)
	assert.Equal(t, "", result.ReplyText)
}

func TestClient_Dialogue_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	pcmData := make([]byte, 16000)
	wavData := wav.PCMToWAV(pcmData, 16000, 1, 16)

	result, err := client.Dialogue(context.Background(), "28:84:85:4b:43:a4", wavData)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "status=500")
}
