package aisaas

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_TTS_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/speech", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake wav audio data"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	body, contentType, err := client.TTS(context.Background(), "tts-model", "hello")

	require.NoError(t, err)
	require.NotNil(t, body)
	assert.Equal(t, "audio/wav", contentType)

	defer body.Close()
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "fake wav audio data", string(data))
}

func TestClient_TTS_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	body, contentType, err := client.TTS(context.Background(), "tts-model", "hello")

	require.Error(t, err)
	assert.Nil(t, body)
	assert.Contains(t, err.Error(), "status=400")
	assert.Equal(t, "", contentType)
}

func TestParseTTSStream_Basic(t *testing.T) {
	fakeAudio := []byte("fake audio content")
	body := io.NopCloser(io.MultiReader(
		&fakeAudioReader{data: fakeAudio},
	))

	chunks, err := ParseTTSStream(body)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, fakeAudio, chunks[0].TTSAudio)
}

type fakeAudioReader struct {
	data []byte
	pos  int
}

func (f *fakeAudioReader) Read(p []byte) (n int, err error) {
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}
	n = copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}
