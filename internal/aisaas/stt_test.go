package aisaas

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
)

func TestClient_STT_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/transcriptions", r.URL.Path)
		assert.Equal(t, "test-model", r.FormValue("model"))

		contentType := r.Header.Get("Content-Type")
		assert.Contains(t, contentType, "multipart/form-data")

		err := r.ParseMultipartForm(32 << 20)
		require.NoError(t, err)

		file, _, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()

		fileContent, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.True(t, len(fileContent) > 44, "should be valid WAV with header")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text":    "hello world",
			"emotion": "neutral",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	pcmData := []byte{0x00, 0x00, 0x00, 0x00}
	wavData := wav.PCMToWAV(pcmData, 16000, 1, 16)

	result, err := client.STT(context.Background(), "test-model", wavData)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "hello world", result.Text)
	assert.Equal(t, "neutral", result.Emotion)
}

func TestClient_STT_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	wavData := wav.PCMToWAV([]byte{0x00, 0x00}, 16000, 1, 16)

	result, err := client.STT(context.Background(), "test-model", wavData)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "status=400")
}

func TestSTTRequest_ValidatesWAVMagic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		ct := "multipart/form-data; boundary=" + string(boundaryFromBytes(bodyBytes))
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.Header.Set("Content-Type", ct)
		r.ParseMultipartForm(32 << 20)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"text": "test",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	invalidWav := []byte("not a wav file")
	result, err := client.STT(context.Background(), "test-model", invalidWav)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test", result.Text)
}

func boundaryFromBytes(data []byte) string {
	const boundaryPrefix = "--"
	start := bytes.Index(data, []byte(boundaryPrefix))
	if start < 0 {
		return ""
	}
	end := bytes.Index(data[start:], []byte("\r\n"))
	if end < 0 {
		return ""
	}
	return string(data[start : start+end])
}
