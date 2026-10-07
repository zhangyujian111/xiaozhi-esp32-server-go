package aisaas

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage) (io.ReadCloser, error) {
	req := ChatRequest{Model: model, Messages: messages, Stream: true}
	r := c.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetDoNotParseResponse(true)
	c.applyAPIKey(r)
	resp, err := r.Post(c.baseURL + "/v1/chat/completions")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		resp.RawBody().Close()
		return nil, fmt.Errorf("llm call failed: status=%d", resp.StatusCode())
	}
	return resp.RawBody(), nil
}

func ParseLLMStream(r io.Reader, onToken func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			return nil
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				if err := onToken(c.Delta.Content); err != nil {
					return err
				}
			}
		}
	}
	return scanner.Err()
}
