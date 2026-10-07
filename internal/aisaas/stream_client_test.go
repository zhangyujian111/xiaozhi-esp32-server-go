package aisaas

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// roundTripperFunc 把 func 适配成 http.RoundTripper（用于注入 fake transport）。
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// streamResp 构造 SSE 响应（直接给 raw SSE bytes 流）。
func streamResp(body string) *http.Response {
	hdr := make(http.Header)
	hdr.Set("Content-Type", "text/event-stream")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// streamStatusResp 非 200 响应。
func streamStatusResp(code int) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("error")),
	}
}

// streamCustomResp 自定义 Content-Type + body。
func streamCustomResp(ct, body string) *http.Response {
	hdr := make(http.Header)
	hdr.Set("Content-Type", ct)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     hdr,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestDialogueStream_HappyPath 验证完整 SSE 序列被正确解析为 events。
func TestDialogueStream_HappyPath(t *testing.T) {
	sseBody := strings.Join([]string{
		"event: tts-start",
		`data: {"msgId":"m1","sampleRate":16000,"channels":1}`,
		"",
		"event: sentence-start",
		`data: {"text":"你好世界","msgId":"m1"}`,
		"",
		"event: tts-audio",
		"data: AQIDBA==",
		"",
		"event: sentence-end",
		`data: {"msgId":"m1"}`,
		"",
		"event: tts-stop",
		`data: {"msgId":"m1"}`,
		"",
		"event: done",
		`data: {}`,
		"",
	}, "\n")

	gotReqCh := make(chan *http.Request, 1)
	sc := &StreamClient{
		BaseURL:       "http://fake",
		InternalToken: "tok",
		APIKey:        "sk-aisaas-test",
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				gotReqCh <- r
				return streamResp(sseBody), nil
			}),
		},
	}

	events, errs := sc.DialogueStream(context.Background(), "dev-001", []byte("wav"))

	var gotReq *http.Request
	select {
	case gotReq = <-gotReqCh:
	case <-time.After(time.Second):
		t.Fatal("no request made within 1s")
	}
	if gotReq.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", gotReq.Method)
	}
	if gotReq.Header.Get("X-Internal-Token") != "tok" {
		t.Errorf("X-Internal-Token = %q, want tok", gotReq.Header.Get("X-Internal-Token"))
	}
	if gotReq.Header.Get("Authorization") != "Bearer sk-aisaas-test" {
		t.Errorf("Authorization = %q, want Bearer sk-aisaas-test", gotReq.Header.Get("Authorization"))
	}

	wantTypes := []string{"tts-start", "sentence-start", "tts-audio", "sentence-end", "tts-stop", "done"}
	var got []DialogueStreamEvent
	for ev := range events {
		got = append(got, ev)
	}
	// errs 必然空（happy path）
	for e := range errs {
		t.Fatalf("unexpected err: %v", e)
	}
	if len(got) != len(wantTypes) {
		t.Fatalf("event count = %d, want %d", len(got), len(wantTypes))
	}
	for i, ev := range got {
		if ev.Type != wantTypes[i] {
			t.Errorf("events[%d].Type = %q, want %q", i, ev.Type, wantTypes[i])
		}
	}
	// tts-audio 第 2 个): Audio 字段被 base64 解码（"AQIDBA==" → 4 bytes 0x01,0x02,0x03,0x04）
	wantAudioBytes := []byte{0x01, 0x02, 0x03, 0x04}
	if string(got[2].Audio) != string(wantAudioBytes) {
		t.Errorf("tts-audio Audio = %v, want %v", got[2].Audio, wantAudioBytes)
	}
	// tts-start 第 0 个): Payload 是 JSON map
	if got[0].Payload["sampleRate"].(float64) != 16000 {
		t.Errorf("tts-start sampleRate = %v, want 16000", got[0].Payload["sampleRate"])
	}
}

// TestDialogueStream_ErrorEvent 验证 aisaas 发 error event 时被解析为 DialogueStreamEvent{Type:error}。
func TestDialogueStream_ErrorEvent(t *testing.T) {
	sseBody := strings.Join([]string{
		"event: tts-start",
		`data: {"msgId":"m1"}`,
		"",
		"event: error",
		`data: {"tag":"llm","message":"upstream timeout"}`,
		"",
	}, "\n")

	sc := &StreamClient{
		BaseURL:       "http://fake",
		InternalToken: "tok",
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return streamResp(sseBody), nil
			}),
		},
	}

	events, _ := sc.DialogueStream(context.Background(), "dev", []byte("wav"))
	var got []DialogueStreamEvent
	for ev := range events {
		got = append(got, ev)
	}

	hasErr := false
	for _, ev := range got {
		if ev.Type == "error" {
			hasErr = true
			if ev.Payload["message"] != "upstream timeout" {
				t.Errorf("error payload message = %v", ev.Payload["message"])
			}
		}
	}
	if !hasErr {
		t.Errorf("expected error event, got %+v", got)
	}
}

// TestDialogueStream_Non200Status 验证 aisaas 返回 5xx 时 errs channel 有错误。
func TestDialogueStream_Non200Status(t *testing.T) {
	sc := &StreamClient{
		BaseURL:       "http://fake",
		InternalToken: "tok",
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return streamStatusResp(http.StatusInternalServerError), nil
			}),
		},
	}

	events, errs := sc.DialogueStream(context.Background(), "dev", []byte("wav"))
	// events 应空
	for range events {
		t.Errorf("unexpected event")
	}
	select {
	case err, ok := <-errs:
		if !ok {
			t.Fatal("errs channel closed unexpectedly")
		}
		if !strings.Contains(err.Error(), "non-200") {
			t.Errorf("err msg = %v, want non-200", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for errs")
	}
}

// TestDialogueStream_WrongContentType 验证 content-type 非 text/event-stream 时报错。
func TestDialogueStream_WrongContentType(t *testing.T) {
	resp := streamCustomResp("application/json", `{"code":0,"data":{"replyText":"hi","audioBase64":"AAAA","audioFormat":"opus","sampleRate":16000,"userText":"hi"}}`)

	sc := &StreamClient{
		BaseURL:       "http://fake",
		InternalToken: "tok",
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return resp, nil
			}),
		},
	}

	events, errs := sc.DialogueStream(context.Background(), "dev", []byte("wav"))
	for range events {
		t.Errorf("should have no events")
	}
	select {
	case err := <-errs:
		if !strings.Contains(err.Error(), "content-type") {
			t.Errorf("err = %v, want content-type", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// TestDialogueStream_ContextCancel 验证 ctx cancel 时 stream goroutine 退出。
func TestDialogueStream_ContextCancel(t *testing.T) {
	// 慢响应: 100 byte SSE body 在 200ms 后关闭
	var closed atomic.Bool
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer closed.Store(true)
		fmt.Fprint(pw, "event: tts-start\ndata: {\"msgId\":\"m1\"}\n\n")
		time.Sleep(500 * time.Millisecond)
		fmt.Fprint(pw, "event: done\ndata: {}\n\n")
	}()

	sc := &StreamClient{
		BaseURL:       "http://fake",
		InternalToken: "tok",
		HTTPClient: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				resp := streamResp("")
				resp.Body = pr
				return resp, nil
			}),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	evCh, errCh := sc.DialogueStream(ctx, "dev", []byte("wav"))

	// 拿第一个 event 后立刻 cancel
	first := <-evCh
	if first.Type != "tts-start" {
		t.Errorf("first event type = %q, want tts-start", first.Type)
	}
	cancel()

	// 等收尾: events 关闭 + errs 关闭 + pr writer 关闭
	for range evCh {
	}
	for range errCh {
	}
	// 给 goroutine 时间清理
	time.Sleep(100 * time.Millisecond)
	if !closed.Load() {
		t.Errorf("pr writer goroutine still running after ctx cancel")
	}
}