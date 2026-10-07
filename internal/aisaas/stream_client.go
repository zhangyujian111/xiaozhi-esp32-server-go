package aisaas

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DialogueStreamEvent 解析 aisaas SSE event 序列。
//
// Type 取自 SSE "event: <type>" 行（tts-start/sentence-start/tts-audio/sentence-end/tts-stop/done/error）。
// Payload 是 event 对应 data JSON object；tts-audio 时 Audio 已 base64 解码为 bytes。
type DialogueStreamEvent struct {
	Type    string
	Payload map[string]any
	Audio   []byte
}

// StreamClient 是 aisaas SSE 流式对话 client。
//
// 内部用裸 http.Client（不走 resty）以便消费流式响应体。
type StreamClient struct {
	BaseURL       string
	InternalToken string
	APIKey        string
	HTTPClient    *http.Client
}

// NewStreamClient 构造 SSE 流式 client（默认 30s timeout，配置 streaming-enabled handler 可覆盖）。
func NewStreamClient(baseURL, internalToken string) *StreamClient {
	return &StreamClient{
		BaseURL:       baseURL,
		InternalToken: internalToken,
		HTTPClient:    &http.Client{Timeout: 0}, // streaming：禁用 timeout（依赖 ctx）
	}
}

// DialogueStream POST /internal/api/v1/dialogue/stream，返回 events channel 和 errs channel。
//
// events channel 由 SSE 解析器写入；SSE 流关闭后 events 关闭。
// errs channel 写入任何致命错误（HTTP 非 200、SSE 解析错误）；读 events 时如伴随 error 应同时检查 errs。
func (s *StreamClient) DialogueStream(ctx context.Context, deviceID string, wavBytes []byte) (<-chan DialogueStreamEvent, <-chan error) {
	events := make(chan DialogueStreamEvent, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		body, err := json.Marshal(map[string]interface{}{
			"deviceId":    deviceID,
			"audioFormat": "wav",
			"sampleRate":  16000,
			"channels":    1,
			"audioBase64": base64.StdEncoding.EncodeToString(wavBytes),
		})
		if err != nil {
			errs <- fmt.Errorf("marshal req: %w", err)
			return
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/internal/api/v1/dialogue/stream", bytes.NewReader(body))
		if err != nil {
			errs <- fmt.Errorf("build req: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Internal-Token", s.InternalToken)
		if s.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+s.APIKey)
		}

		resp, err := s.HTTPClient.Do(httpReq)
		if err != nil {
			errs <- fmt.Errorf("dialogue stream request: %w", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errs <- fmt.Errorf("dialogue stream non-200: status=%d", resp.StatusCode)
			return
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
			errs <- fmt.Errorf("unexpected content-type %q (expected text/event-stream)", ct)
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		// 放大 buffer：tts-audio 单个 data 可能 > 64KB（base64 PCM chunk）。
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		var cur map[string]string
		flush := func() {
			if cur == nil {
				return
			}
			ev, ok := parseEvent(cur)
			if ok {
				select {
				case <-ctx.Done():
					return
				case events <- ev:
				}
			}
			cur = nil
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				if cur == nil {
					cur = map[string]string{}
				}
				cur["event"] = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				if cur == nil {
					cur = map[string]string{}
				}
				cur["data"] = strings.TrimPrefix(line, "data: ")
			} else if strings.HasPrefix(line, ":") {
				// SSE comment (heartbeat)；不动
				continue
			}
		}
		// scanner EOF: flush 残留 event（最后一行 data 后通常有 \n\n 但 bufio
		// 读到 EOF 时最后一次空行可能没被 Scan 看见——这里兜底 flush）
		flush()
		if err := scanner.Err(); err != nil {
			errs <- fmt.Errorf("sse read: %w", err)
		}
	}()

	return events, errs
}

// parseEvent 把 "event"/"data" 字典转 DialogueStreamEvent。
//
// tts-audio 的 data 是 base64 字符串；解到 Audio 字段以便 orchestrator 直接 opus decode。
// 其余 event 的 data 是 JSON object，落 Payload map。
func parseEvent(m map[string]string) (DialogueStreamEvent, bool) {
	typ, ok := m["event"]
	if !ok {
		return DialogueStreamEvent{}, false
	}
	raw, ok := m["data"]
	if !ok {
		return DialogueStreamEvent{}, false
	}

	ev := DialogueStreamEvent{Type: typ}
	if typ == "tts-audio" {
		// aisaas writes base64 string for tts-audio data
		ev.Audio, _ = base64.StdEncoding.DecodeString(raw)
		return ev, true
	}
	// 其余 event：data 是 JSON object
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		// malformed payload 也照常 send，让 caller 处理（不会 fatal）
		ev.Payload = map[string]any{"_parse_error": err.Error(), "_raw": raw}
		return ev, true
	}
	ev.Payload = payload
	return ev, true
}