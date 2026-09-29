package aisaas

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

type TTSRequest struct {
	Model  string `json:"model"`
	Input  string `json:"input"`
	Stream bool   `json:"stream"`
	Voice  string `json:"voice,omitempty"`
}

type TTSChunk struct {
	TTSStart bool
	TTSAudio []byte
	TTSEnd   bool
}

func (c *Client) TTS(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
	req := TTSRequest{Model: model, Input: text, Stream: true}
	resp, err := c.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetDoNotParseResponse(true).
		Post(c.baseURL + "/v1/audio/speech")
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode() != http.StatusOK {
		resp.RawBody().Close()
		return nil, "", fmt.Errorf("tts failed: status=%d", resp.StatusCode())
	}
	return resp.RawBody(), resp.Header().Get("Content-Type"), nil
}

func ParseTTSStream(r io.Reader) ([]TTSChunk, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return []TTSChunk{{TTSAudio: data}}, nil
}
