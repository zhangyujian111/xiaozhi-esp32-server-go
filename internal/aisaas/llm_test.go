package aisaas

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLLMStream_BasicChinese(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"content":"今天"}}]}

data: {"choices":[{"delta":{"content":"是"}}]}

data: [DONE]

`
	var tokens []string
	err := ParseLLMStream(strings.NewReader(sseData), func(token string) error {
		tokens = append(tokens, token)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"今天", "是"}, tokens)
}

func TestParseLLMStream_MultiChoice(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"content":"Hello"}},{"delta":{"content":"World"}}]}

data: [DONE]

`
	var tokens []string
	err := ParseLLMStream(strings.NewReader(sseData), func(token string) error {
		tokens = append(tokens, token)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Hello", "World"}, tokens)
}

func TestParseLLMStream_MalformedChunkSkipped(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"content":"valid"}}]}

data: {bad json

data: {"choices":[{"delta":{"content":"after"}}]}

data: [DONE]

`
	var tokens []string
	err := ParseLLMStream(strings.NewReader(sseData), func(token string) error {
		tokens = append(tokens, token)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"valid", "after"}, tokens)
}

func TestParseLLMStream_OnTokenErrorPropagated(t *testing.T) {
	sseData := `data: {"choices":[{"delta":{"content":"token1"}}]}

data: {"choices":[{"delta":{"content":"token2"}}]}

data: [DONE]

`
	expectedErr := assert.AnError
	var tokens []string
	err := ParseLLMStream(strings.NewReader(sseData), func(token string) error {
		tokens = append(tokens, token)
		if token == "token2" {
			return expectedErr
		}
		return nil
	})
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, []string{"token1", "token2"}, tokens)
}

func TestParseLLMStream_LargeToken(t *testing.T) {
	largeContent := strings.Repeat("中", 70000)
	sseData := `data: {"choices":[{"delta":{"content":"` + largeContent + `"}}]}

data: [DONE]

`
	var tokens []string
	err := ParseLLMStream(strings.NewReader(sseData), func(token string) error {
		tokens = append(tokens, token)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, largeContent, tokens[0])
}

func TestClient_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`data: {"choices":[{"delta":{"content":"今天"}}]}

`))
		w.Write([]byte(`data: {"choices":[{"delta":{"content":"是"}}]}

`))
		w.Write([]byte(`data: [DONE]

`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	body, err := client.Chat(context.Background(), "test-model", []ChatMessage{{Role: "user", Content: "hello"}})
	require.NoError(t, err)
	require.NotNil(t, body)

	defer body.Close()

	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Contains(t, string(data), "今天")
	assert.Contains(t, string(data), "是")
	assert.Contains(t, string(data), "[DONE]")
}

func TestClient_Chat_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token")
	body, err := client.Chat(context.Background(), "test-model", nil)

	require.Error(t, err)
	assert.Nil(t, body)
	assert.Contains(t, err.Error(), "status=400")
}
