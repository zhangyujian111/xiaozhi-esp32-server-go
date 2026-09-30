package dialogue

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/opus"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type mockMemory struct {
	GetWindowFunc func(ctx context.Context, deviceID string, n int) ([]store.Message, error)
	SaveTurnFunc  func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error
}

func (m *mockMemory) GetWindow(ctx context.Context, deviceID string, n int) ([]store.Message, error) {
	if m.GetWindowFunc != nil {
		return m.GetWindowFunc(ctx, deviceID, n)
	}
	return nil, nil
}

func (m *mockMemory) SaveTurn(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
	if m.SaveTurnFunc != nil {
		return m.SaveTurnFunc(ctx, deviceID, sessionID, userText, assistantText)
	}
	return nil
}

type mockOpusEncoder struct {
	EncodeFunc func(pcm []int16, frameSize int) ([]byte, error)
}

func (m *mockOpusEncoder) Encode(pcm []int16, frameSize int) ([]byte, error) {
	if m.EncodeFunc != nil {
		return m.EncodeFunc(pcm, frameSize)
	}
	return []byte("fake-opus-data"), nil
}

type mockConn struct {
	WriteJSONFunc    func(v interface{}) error
	WriteMessageFunc func(msgType int, data []byte) error
	CloseFunc        func() error
}

func (m *mockConn) WriteJSON(v interface{}) error {
	if m.WriteJSONFunc != nil {
		return m.WriteJSONFunc(v)
	}
	return nil
}

func (m *mockConn) WriteMessage(msgType int, data []byte) error {
	if m.WriteMessageFunc != nil {
		return m.WriteMessageFunc(msgType, data)
	}
	return nil
}

func (m *mockConn) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

type mockAisaas struct {
	STTFunc  func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error)
	ChatFunc func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error)
	TTSFunc  func(ctx context.Context, model, text string) (io.ReadCloser, string, error)
	GetPersonaFunc func(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error)
}

func (m *mockAisaas) STT(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
	if m.STTFunc != nil {
		return m.STTFunc(ctx, model, wavData)
	}
	return &aisaas.STTResult{Text: "mock transcription"}, nil
}

func (m *mockAisaas) Chat(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
	if m.ChatFunc != nil {
		return m.ChatFunc(ctx, model, messages)
	}
	return io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"Hello world"}}]}` + "\ndata: [DONE]")), nil
}

func (m *mockAisaas) TTS(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
	if m.TTSFunc != nil {
		return m.TTSFunc(ctx, model, text)
	}
	return io.NopCloser(strings.NewReader("fake-tts-audio")), "audio/wav", nil
}

func (m *mockAisaas) GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error) {
	if m.GetPersonaFunc != nil {
		return m.GetPersonaFunc(ctx, deviceID, tenantID)
	}
	return nil, aisaas.ErrPersonaNotBound
}

func TestOrchestrator_Run_SuccessFlow(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		GetWindowFunc: func(ctx context.Context, deviceID string, n int) ([]store.Message, error) {
			return nil, nil
		},
		SaveTurnFunc: func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
			return nil
		},
	}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte("hello-world-bytes"), nil
		},
	}

	var ttsMessages []string
	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			if msg, ok := v.(protocol.TTSMessage); ok {
				ttsMessages = append(ttsMessages, string(msg.State))
			}
			return nil
		},
	}

	ac := &mockAisaas{}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{
		DeviceID:    "device1",
		LLMConfigID: 1,
		TTSConfigID: 1,
		VoiceName:   "alice",
	}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	require.True(t, len(ttsMessages) >= 1, "expected at least one TTS message, got %d", len(ttsMessages))
}

func TestOrchestrator_Run_STTError(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return nil, errors.New("stt failed")
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.Contains(t, err.Error(), "stt")
}

func TestOrchestrator_Run_OpusEncodeError(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return nil, errors.New("opus encode failed")
		},
	}

	conn := &mockConn{}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return &aisaas.STTResult{Text: "hello"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"Hello."}}]}` + "\ndata: [DONE]")), nil
		},
		TTSFunc: func(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("fake-tts")), "audio/wav", nil
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.Contains(t, err.Error(), "opus encode")
}

func TestOrchestrator_Run_CtxCanceled(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			return &aisaas.STTResult{Text: "test"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestOrchestrator_Run_MultipleSentences(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		SaveTurnFunc: func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
			return nil
		},
	}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte("encoded"), nil
		},
	}

	var sentenceCount int
	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			if msg, ok := v.(protocol.TTSMessage); ok {
				if msg.State == protocol.TTSStateSentenceStart {
					sentenceCount++
				}
			}
			return nil
		},
	}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return &aisaas.STTResult{Text: "hello"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(
				`data: {"choices":[{"delta":{"content":"第一句。"}}]}` + "\n" +
					`data: {"choices":[{"delta":{"content":"第二句。"}}]}` + "\n" +
					`data: [DONE]`)), nil
		},
		TTSFunc: func(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("fake-tts")), "audio/wav", nil
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	require.Equal(t, 2, sentenceCount, "expected 2 sentences")
}

func TestOrchestrator_Run_EmptyAudio(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}
	ac := &mockAisaas{}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	session := "session1" // unused; orchestrator takes audioBytes directly
	_ = session

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
}

func TestOrchestrator_Run_ChatError(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return &aisaas.STTResult{Text: "hello"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			return nil, errors.New("chat failed")
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.Contains(t, err.Error(), "chat")
}

func TestOrchestrator_Run_TTSError(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}

	conn := &mockConn{}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return &aisaas.STTResult{Text: "hello"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(
				`data: {"choices":[{"delta":{"content":"测试。"}}]}` + "\ndata: [DONE]")), nil
		},
		TTSFunc: func(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
			return nil, "", errors.New("tts failed")
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.Contains(t, err.Error(), "tts")
}

func TestOrchestrator_Run_MemorySaveError(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		SaveTurnFunc: func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
			return errors.New("save failed")
		},
	}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte("encoded"), nil
		},
	}

	var ttsMessages []string
	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			if msg, ok := v.(protocol.TTSMessage); ok {
				ttsMessages = append(ttsMessages, string(msg.State))
			}
			return nil
		},
	}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return &aisaas.STTResult{Text: "hello"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(
				`data: {"choices":[{"delta":{"content":"回复。"}}]}` + "\ndata: [DONE]")), nil
		},
		TTSFunc: func(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("fake-tts")), "audio/wav", nil
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
}

func TestOrchestrator_Run_WriteJSONError(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte("encoded"), nil
		},
	}

	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			return errors.New("write failed")
		},
	}

	ac := &mockAisaas{
		STTFunc: func(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error) {
			return &aisaas.STTResult{Text: "hello"}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(
				`data: {"choices":[{"delta":{"content":"测试。"}}]}` + "\ndata: [DONE]")), nil
		},
		TTSFunc: func(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
			return io.NopCloser(strings.NewReader("fake-tts")), "audio/wav", nil
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	_ = "session1" // orchestrator takes audioBytes directly; sessionID is a plain string

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.Contains(t, err.Error(), "write failed")
}

func TestOrchestrator_Run_InjectsPersonaSystemPrompt(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		GetWindowFunc: func(ctx context.Context, deviceID string, n int) ([]store.Message, error) {
			return nil, nil
		},
		SaveTurnFunc: func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
			return nil
		},
	}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	const wantSystem = "You are Hakumi. Hakumi is a small life form from a distant planet."
	var capturedMessages []aisaas.ChatMessage
	var capturedModel string
	ac := &mockAisaas{
		GetPersonaFunc: func(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error) {
			assert.Equal(t, "device1", deviceID)
			assert.Equal(t, int64(1001), tenantID)
			return &aisaas.Persona{
				ID:             1,
				Code:           "hakumi02",
				Name:           "hakumi",
				SystemPrompt:   wantSystem,
				DefaultModelID: "demo-chat",
				TenantID:       1001,
			}, nil
		},
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			capturedMessages = messages
			capturedModel = model
			return io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"hi"}}]}` + "\ndata: [DONE]")), nil
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	err := orch.Run(context.Background(), "session1", conn, &aisaas.DeviceInfo{
		DeviceID: "device1",
		UserID:   1001,
	}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	require.Len(t, capturedMessages, 2, "system + user")
	assert.Equal(t, "system", capturedMessages[0].Role)
	assert.Equal(t, wantSystem, capturedMessages[0].Content)
	assert.Equal(t, "user", capturedMessages[1].Role)
	assert.Equal(t, "demo-chat", capturedModel, "use persona.DefaultModelID")
}

func TestOrchestrator_Run_PersonaNotBound_FallsBackToDefaultModel(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		GetWindowFunc: func(ctx context.Context, deviceID string, n int) ([]store.Message, error) {
			return nil, nil
		},
	}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	var capturedMessages []aisaas.ChatMessage
	var capturedModel string
	ac := &mockAisaas{
		// GetPersonaFunc nil → 默认返回 ErrPersonaNotBound
		ChatFunc: func(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error) {
			capturedMessages = messages
			capturedModel = model
			return io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"hi"}}]}` + "\ndata: [DONE]")), nil
		},
	}
	dec, _ := opus.NewDecoder(24000, 1)
	orch := NewOrchestrator(ac, splitter, mem, dec, enc, zerolog.Logger{})

	err := orch.Run(context.Background(), "session1", conn, &aisaas.DeviceInfo{
		DeviceID:    "device1",
		LLMConfigID: 7,
	}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	require.Len(t, capturedMessages, 1, "无 system，仅 user")
	assert.Equal(t, "user", capturedMessages[0].Role)
	assert.Equal(t, "llm-7", capturedModel, "fallback to LLMConfigID")
}
