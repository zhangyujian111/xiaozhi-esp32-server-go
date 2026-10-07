package dialogue

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

func wavFromPCM(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	return wav.PCMToWAV(pcm, sampleRate, channels, bitsPerSample)
}

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

// New AisaasClient interface using Dialogue() instead of STT/Chat/TTS
type mockAisaasDialogue struct {
	DialogueFunc         func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error)
	GetPersonaFunc       func(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error)
}

func (m *mockAisaasDialogue) Dialogue(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
	if m.DialogueFunc != nil {
		return m.DialogueFunc(ctx, deviceID, wavBytes)
	}
	return &aisaas.DialogueResult{
		UserText:    "test user text",
		ReplyText:   "test reply text",
		AudioFormat: "opus",
		SampleRate:  24000,
		Audio:       []byte("test-opus-audio"),
	}, nil
}

func (m *mockAisaasDialogue) GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error) {
	if m.GetPersonaFunc != nil {
		return m.GetPersonaFunc(ctx, deviceID, tenantID)
	}
	return nil, aisaas.ErrPersonaNotBound
}

// Tests below use the new Dialogue-based interface

func TestOrchestrator_Run_Dialogue_HappyPath(t *testing.T) {
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
	var writeMsgData []byte
	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			if msg, ok := v.(protocol.TTSMessage); ok {
				ttsMessages = append(ttsMessages, string(msg.State))
			}
			return nil
		},
		WriteMessageFunc: func(msgType int, data []byte) error {
			writeMsgData = data
			return nil
		},
	}

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return &aisaas.DialogueResult{
				UserText:    "今天天气怎么样",
				ReplyText:   "今天天气很好",
				AudioFormat: "opus",
				SampleRate:  24000,
				Audio:       []byte("test-opus-response"),
			}, nil
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{
		DeviceID: "28:84:85:4b:43:a4",
	}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	assert.NotEmpty(t, ttsMessages, "expected TTS messages")
	assert.Equal(t, []byte("test-opus-response"), writeMsgData)
}

func TestOrchestrator_Run_Dialogue_EmptyAudio(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}
	ac := &mockAisaasDialogue{}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{})

	require.NoError(t, err, "empty audio should be no-op")
}

func TestOrchestrator_Run_Dialogue_Error(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return nil, errors.New("dialogue failed")
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dialogue")
}

func TestOrchestrator_Run_Dialogue_DeviceNotFound(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return nil, aisaas.ErrDeviceNotRegistered
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "unknown"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "device not registered")
}

func TestOrchestrator_Run_Dialogue_QuotaExceeded(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{}
	enc := &mockOpusEncoder{}
	conn := &mockConn{}

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return nil, errors.New("quota exceeded")
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota")
}

func TestOrchestrator_Run_Dialogue_WriteJSONError(t *testing.T) {
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

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return &aisaas.DialogueResult{
				UserText:    "hello",
				ReplyText:   "hi",
				AudioFormat: "opus",
				SampleRate:  24000,
				Audio:       []byte("test-opus"),
			}, nil
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.Error(t, err)
	require.Contains(t, err.Error(), "write failed")
}

func TestOrchestrator_Run_Dialogue_PlayOpusToClient(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		SaveTurnFunc: func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
			return nil
		},
	}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte("encoded-opus-frame"), nil
		},
	}

	var receivedOpusFrame []byte
	var ttsStartSent bool
	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			if msg, ok := v.(protocol.TTSMessage); ok {
				if msg.State == protocol.TTSStateStart {
					ttsStartSent = true
				}
			}
			return nil
		},
		WriteMessageFunc: func(msgType int, data []byte) error {
			receivedOpusFrame = data
			return nil
		},
	}

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return &aisaas.DialogueResult{
				UserText:    "hello",
				ReplyText:   "hi there",
				AudioFormat: "opus",
				SampleRate:  24000,
				Audio:       []byte("raw-opus-bytes"),
			}, nil
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	assert.True(t, ttsStartSent, "TTS start should be sent")
	assert.Equal(t, []byte("raw-opus-bytes"), receivedOpusFrame, "raw opus should be sent directly")
}

func TestOrchestrator_Run_Dialogue_PlayOpusToClientWithWAV(t *testing.T) {
	splitter := NewSentenceSplitter()
	mem := &mockMemory{
		SaveTurnFunc: func(ctx context.Context, deviceID, sessionID, userText, assistantText string) error {
			return nil
		},
	}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte("encoded-opus-frame"), nil
		},
	}

	var receivedOpusFrames [][]byte
	conn := &mockConn{
		WriteJSONFunc: func(v interface{}) error {
			return nil
		},
		WriteMessageFunc: func(msgType int, data []byte) error {
			if msgType == 2 { // binary
				receivedOpusFrames = append(receivedOpusFrames, data)
			}
			return nil
		},
	}

	// Simulate aisaas returning a 120ms valid WAV (16kHz mono 16-bit) -> 2 opus frames at 60ms each.
	const sampleRate = 16000
	const channels = 1
	const bitsPerSample = 16
	const durationMS = 120
	pcmSamples := make([]int16, sampleRate*durationMS/1000)
	pcmBytes := make([]byte, len(pcmSamples)*2)
	for i, s := range pcmSamples {
		binary.LittleEndian.PutUint16(pcmBytes[2*i:], uint16(s))
	}
	wavAudio := wavFromPCM(pcmBytes, sampleRate, channels, bitsPerSample)

	ac := &mockAisaasDialogue{
		DialogueFunc: func(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error) {
			return &aisaas.DialogueResult{
				UserText:    "test",
				ReplyText:   "test reply",
				AudioFormat: "wav",
				SampleRate:  sampleRate,
				Audio:       wavAudio,
			}, nil
		},
	}
	orch := NewOrchestrator(ac, splitter, mem, enc, zerolog.Logger{})

	ctx := context.Background()
	err := orch.Run(ctx, "session1", conn, &aisaas.DeviceInfo{DeviceID: "device1"}, []byte{0, 0, 0, 0})

	require.NoError(t, err)
	// aisaas returns wav -> orchestrator decodes to PCM and encodes 2 opus frames (60ms each).
	assert.Len(t, receivedOpusFrames, 2, "expected 2 opus frames (120ms / 60ms)")
	for i, frame := range receivedOpusFrames {
		assert.Equal(t, []byte("encoded-opus-frame"), frame, "frame %d content", i)
	}
}
