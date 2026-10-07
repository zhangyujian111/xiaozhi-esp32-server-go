package dialogue

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
)

func ptrInt(v int) *int { return &v }

// mockStreamClient returns a fixed event list (with optional early error).
type mockStreamClient struct {
	events    []aisaas.DialogueStreamEvent
	errBefore *int
}

// recordingConn extends mockConn: captures every write for assertions.
type recordingConn struct {
	mockConn
	gotMessages     [][]byte
	gotMessageTypes []int
	gotJSONPayloads []interface{}
}

func (r *recordingConn) WriteMessage(msgType int, data []byte) error {
	r.gotMessages = append(r.gotMessages, append([]byte{}, data...))
	r.gotMessageTypes = append(r.gotMessageTypes, msgType)
	return nil
}

func (r *recordingConn) WriteJSON(v interface{}) error {
	r.gotJSONPayloads = append(r.gotJSONPayloads, v)
	return nil
}

func (m *mockStreamClient) DialogueStream(ctx context.Context, deviceID string, wavBytes []byte) (<-chan aisaas.DialogueStreamEvent, <-chan error) {
	events := make(chan aisaas.DialogueStreamEvent, len(m.events)+1)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		if m.errBefore != nil && *m.errBefore >= 0 && *m.errBefore < len(m.events) {
			for i := 0; i < *m.errBefore; i++ {
				events <- m.events[i]
			}
			events <- aisaas.DialogueStreamEvent{
				Type:    "error",
				Payload: map[string]any{"tag": "test", "message": "stream aborted"},
			}
			return
		}
		for _, e := range m.events {
			select {
			case <-ctx.Done():
				return
			case events <- e:
			}
		}
	}()
	return events, errs
}

// TestStreamOrchestrator_HappyPath verifies SSE events flow into ws conn writes.
//
// tts-audio event 的 Audio 字段是 60ms raw int16 LE PCM @ 16kHz（1920 bytes = 960 samples）；
// orchestrator 收到后调 opusEnc.Encode → WriteAudioFrame 推 ESP32。
func TestStreamOrchestrator_HappyPath(t *testing.T) {
	opusFrame := []byte{0xAB, 0xCD, 0xEF, 0x12}
	pcmChunk := make([]byte, 1920) // 60ms @ 16kHz mono int16 LE

	client := &mockStreamClient{
		events: []aisaas.DialogueStreamEvent{
			{Type: "tts-start", Payload: map[string]any{"msgId": "m1", "sampleRate": 16000}},
			{Type: "sentence-start", Payload: map[string]any{"msgId": "m1", "text": "hello"}},
			{Type: "tts-audio", Audio: pcmChunk},
			{Type: "tts-audio", Audio: pcmChunk},
			{Type: "sentence-end", Payload: map[string]any{"msgId": "m1"}},
			{Type: "tts-stop", Payload: map[string]any{"msgId": "m1"}},
			{Type: "done", Payload: map[string]any{}},
		},
	}

	opusEnc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			if frameSize != 960 {
				t.Errorf("opusEnc.Encode frameSize = %d, want 960", frameSize)
			}
			if len(pcm) != 960 {
				t.Errorf("opusEnc.Encode pcm len = %d, want 960", len(pcm))
			}
			return opusFrame, nil
		},
	}

	conn := &recordingConn{}
	so := &StreamOrchestrator{
		framePacer: noopFramePacer{},
		opusEnc:    opusEnc,
	}
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return client })

	if err := so.Run(context.Background(), "dev-001", []byte("pcm"), conn, "1", 16000, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := len(conn.gotMessages); got < 2 {
		t.Errorf("binary write count = %d, want >= 2", got)
	}
	for _, msg := range conn.gotMessages {
		if string(msg) != string(opusFrame) {
			t.Errorf("opus payload = %v, want %v", msg, opusFrame)
		}
	}
	if got := len(conn.gotJSONPayloads); got < 4 {
		t.Errorf("JSON write count = %d, want >= 4", got)
	}
}

// TestStreamOrchestrator_V2Header verifies v2 BE header wraps opus payload.
func TestStreamOrchestrator_V2Header(t *testing.T) {
	opusFrame := []byte{0xAB, 0xCD}
	pcmChunk := make([]byte, 1920)
	client := &mockStreamClient{
		events: []aisaas.DialogueStreamEvent{
			{Type: "tts-start", Payload: map[string]any{"msgId": "m1"}},
			{Type: "sentence-start", Payload: map[string]any{"msgId": "m1", "text": "hi"}},
			{Type: "tts-audio", Audio: pcmChunk},
			{Type: "sentence-end", Payload: map[string]any{"msgId": "m1"}},
			{Type: "tts-stop", Payload: map[string]any{"msgId": "m1"}},
			{Type: "done"},
		},
	}

	opusEnc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return opusFrame, nil
		},
	}

	conn := &recordingConn{}
	so := &StreamOrchestrator{framePacer: noopFramePacer{}, opusEnc: opusEnc}
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return client })

	if err := so.Run(context.Background(), "dev", []byte("pcm"), conn, "2", 16000, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var found []byte
	for _, msg := range conn.gotMessages {
		if len(msg) == 16+len(opusFrame) {
			found = msg
			break
		}
	}
	if found == nil {
		t.Fatalf("no v2 frame found, msgs=%d", len(conn.gotMessages))
	}
	if v := binary.BigEndian.Uint16(found[0:2]); v != 2 {
		t.Errorf("v2 version = %d, want 2", v)
	}
	if v := binary.BigEndian.Uint32(found[12:16]); v != uint32(len(opusFrame)) {
		t.Errorf("v2 payload_size = %d, want %d", v, len(opusFrame))
	}
}

// TestStreamOrchestrator_ErrorEvent verifies error event triggers graceful exit
// without writing a type=error JSON payload (ESP32 client cannot parse that
// message type, so the orchestrator must only log and stop).
func TestStreamOrchestrator_ErrorEvent(t *testing.T) {
	client := &mockStreamClient{
		events: []aisaas.DialogueStreamEvent{
			{Type: "tts-start", Payload: map[string]any{"msgId": "m1"}},
			{Type: "error", Payload: map[string]any{"message": "upstream timeout"}},
		},
		errBefore: ptrInt(1),
	}

	conn := &recordingConn{}
	so := &StreamOrchestrator{framePacer: noopFramePacer{}}
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return client })
	err := so.Run(context.Background(), "dev", []byte("pcm"), conn, "1", 16000, "")
	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	for _, payload := range conn.gotJSONPayloads {
		if m, ok := payload.(map[string]any); ok {
			if m["type"] == "error" {
				t.Fatalf("did not expect type=error JSON payload (ESP32 cannot parse), got %v", conn.gotJSONPayloads)
			}
		}
	}
	if got := len(conn.gotJSONPayloads); got != 1 {
		t.Fatalf("expected only the tts-start payload before exit, got %d payloads: %v", got, conn.gotJSONPayloads)
	}
}

// TestStreamOrchestrator_ContextCancel verifies ctx cancel exits Run.
func TestStreamOrchestrator_ContextCancel(t *testing.T) {
	pcmChunk := make([]byte, 1920) // 60ms PCM @ 16kHz
	prod := func(ctx context.Context) (<-chan aisaas.DialogueStreamEvent, <-chan error) {
		events := make(chan aisaas.DialogueStreamEvent, 1)
		errs := make(chan error, 1)
		var closed atomic.Bool
		go func() {
			defer close(events)
			defer close(errs)
			events <- aisaas.DialogueStreamEvent{Type: "tts-start", Payload: map[string]any{"msgId": "m1"}}
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			i := 0
			for {
				select {
				case <-ctx.Done():
					closed.Store(true)
					return
				case <-ticker.C:
					i++
					select {
					case events <- aisaas.DialogueStreamEvent{Type: "tts-audio", Audio: pcmChunk}:
					default:
					}
					if i >= 5 {
						return
					}
				}
			}
		}()
		_ = closed
		return events, errs
	}

	slow := &slowMockStream{prod: prod}

	conn := &recordingConn{}
	so := &StreamOrchestrator{
		framePacer: noopFramePacer{},
		opusEnc:    &mockOpusEncoder{}, // 返 fake-opus-data
	}
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return slow })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- so.Run(ctx, "dev", []byte("pcm"), conn, "1", 16000, "")
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s after ctx cancel")
	}
}

// TestStreamOrchestrator_PCMBytesToOpusEncode 验证 PCM→opus encode 真实转换：opusEnc 收到
// 960 个 int16 samples（来自 1920 bytes PCM），返 bytes 后由 WriteAudioFrame 推 ESP32。
func TestStreamOrchestrator_PCMBytesToOpusEncode(t *testing.T) {
	// 60ms @ 16kHz mono int16 LE = 1920 bytes = 960 samples
	pcmBytes := make([]byte, 1920)
	for i := range pcmBytes {
		pcmBytes[i] = byte(i % 256) // non-zero values to make sure bytes→int16 conversion produces non-zero
	}
	expectedOpus := []byte{0xCA, 0xFE, 0xBA, 0xBE}

	var receivedSamples []int16
	var receivedFrameSize int
	opusEnc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			receivedSamples = append([]int16{}, pcm...)
			receivedFrameSize = frameSize
			return expectedOpus, nil
		},
	}

	client := &mockStreamClient{
		events: []aisaas.DialogueStreamEvent{
			{Type: "tts-start", Payload: map[string]any{"msgId": "m1"}},
			{Type: "tts-audio", Audio: pcmBytes},
			{Type: "done", Payload: map[string]any{}},
		},
	}

	conn := &recordingConn{}
	so := &StreamOrchestrator{framePacer: noopFramePacer{}, opusEnc: opusEnc}
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return client })

	if err := so.Run(context.Background(), "dev", []byte("pcm"), conn, "1", 16000, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if receivedFrameSize != 960 {
		t.Errorf("opusEnc.Encode frameSize = %d, want 960 (60ms @ 16kHz)", receivedFrameSize)
	}
	if len(receivedSamples) != 960 {
		t.Fatalf("opusEnc.Encode pcm samples = %d, want 960", len(receivedSamples))
	}
	// 验证前 2 个 int16 samples = (byte0, byte1) LE
	wantFirst := int16(pcmBytes[0]) | int16(pcmBytes[1])<<8
	if receivedSamples[0] != wantFirst {
		t.Errorf("opusEnc.Encode samples[0] = %d, want %d (LE int16)", receivedSamples[0], wantFirst)
	}
	// 验证 WriteAudioFrame 收到的是 opus 编码后的 bytes（不是 raw PCM）
	if len(conn.gotMessages) != 1 {
		t.Fatalf("binary write count = %d, want 1", len(conn.gotMessages))
	}
	if string(conn.gotMessages[0]) != string(expectedOpus) {
		t.Errorf("ws binary payload = %v, want opus-encoded bytes %v", conn.gotMessages[0], expectedOpus)
	}
}

// TestStreamOrchestrator_NoOpusEnc 验证 opusEnc 为 nil 时 tts-audio 静默丢弃（debug 模式）。
func TestStreamOrchestrator_NoOpusEnc(t *testing.T) {
	pcmBytes := make([]byte, 1920)
	client := &mockStreamClient{
		events: []aisaas.DialogueStreamEvent{
			{Type: "tts-start", Payload: map[string]any{"msgId": "m1"}},
			{Type: "tts-audio", Audio: pcmBytes},
			{Type: "done", Payload: map[string]any{}},
		},
	}

	conn := &recordingConn{}
	so := &StreamOrchestrator{framePacer: noopFramePacer{}} // 故意不注入 opusEnc
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return client })

	if err := so.Run(context.Background(), "dev", []byte("pcm"), conn, "1", 16000, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(conn.gotMessages); got != 0 {
		t.Errorf("expected 0 binary writes without opusEnc, got %d", got)
	}
}

// slowMockStream wraps prod() func.
type slowMockStream struct {
	prod func(ctx context.Context) (<-chan aisaas.DialogueStreamEvent, <-chan error)
}

func (s *slowMockStream) DialogueStream(ctx context.Context, deviceID string, wavBytes []byte) (<-chan aisaas.DialogueStreamEvent, <-chan error) {
	return s.prod(ctx)
}

// TestStreamOrchestrator_WAVHeaderFormat verifies pcmToWAV output is a valid WAV header.
func TestStreamOrchestrator_WAVHeaderFormat(t *testing.T) {
	pcm := []byte{0x00, 0x01, 0x02, 0x03}
	wav := pcmToWAV(pcm, 16000)
	if len(wav) != 44+4 {
		t.Fatalf("wav len = %d, want 48", len(wav))
	}
	if string(wav[0:4]) != "RIFF" {
		t.Errorf("wav[0:4] = %q, want RIFF", wav[0:4])
	}
	if string(wav[8:12]) != "WAVE" {
		t.Errorf("wav[8:12] = %q, want WAVE", wav[8:12])
	}
	if sr := binary.LittleEndian.Uint32(wav[24:28]); sr != 16000 {
		t.Errorf("sample_rate = %d, want 16000", sr)
	}
	if ch := binary.LittleEndian.Uint16(wav[22:24]); ch != 1 {
		t.Errorf("channels = %d, want 1", ch)
	}
}

// TestStreamOrchestrator_TranslatesSttEvent V8 RED: 验证 aisaas stt event 翻译为 ESP32 JSON。
func TestStreamOrchestrator_TranslatesSttEvent(t *testing.T) {
	client := &mockStreamClient{
		events: []aisaas.DialogueStreamEvent{
			{Type: "stt", Payload: map[string]any{"text": "你好"}},
		},
	}

	conn := &recordingConn{}
	so := &StreamOrchestrator{
		framePacer: noopFramePacer{},
	}
	so.setStreamClientFactory(func(apiKey string) streamClientIface { return client })

	if err := so.Run(context.Background(), "dev-stt", []byte("pcm"), conn, "1", 16000, ""); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 验证 WriteJSON 被调用，且 payload 包含 type:"stt" 和 text:"你好"
	if len(conn.gotJSONPayloads) == 0 {
		t.Fatalf("WriteJSON was never called")
	}
	found := false
	for _, p := range conn.gotJSONPayloads {
		if m, ok := p.(map[string]any); ok {
			if m["type"] == "stt" && m["text"] == "你好" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("WriteJSON payloads = %+v, want one with type=stt and text=你好", conn.gotJSONPayloads)
	}
}

// Compile-time interface checks
var (
	_ streamClientIface = (*mockStreamClient)(nil)
	_ streamClientIface = (*slowMockStream)(nil)
)

// Suppress unused import warning.
var _ = zerolog.Nop