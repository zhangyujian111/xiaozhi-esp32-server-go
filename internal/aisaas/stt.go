package aisaas

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

type STTResult struct {
	Text    string `json:"text"`
	Emotion string `json:"emotion,omitempty"`
}

func (c *Client) STT(ctx context.Context, model string, wavData []byte) (*STTResult, error) {
	var result STTResult
	r := c.http.R().
		SetContext(ctx).
		SetFileReader("file", "audio.wav", bytes.NewReader(wavData)).
		SetFormData(map[string]string{"model": model}).
		SetResult(&result)
	c.applyAPIKey(r)
	resp, err := r.Post(c.baseURL + "/v1/audio/transcriptions")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("stt failed: status=%d body=%s", resp.StatusCode(), resp.String())
	}
	return &result, nil
}
