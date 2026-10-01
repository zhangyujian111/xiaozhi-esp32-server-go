package dialogue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/opus"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

var ErrNotImplemented = errors.New("not implemented")

type AisaasClient interface {
	STT(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error)
	Chat(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error)
	TTS(ctx context.Context, model, text string) (io.ReadCloser, string, error)
	GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error)
}

type OpusEncoder interface {
	Encode(pcm []int16, frameSize int) ([]byte, error)
}

type Conn interface {
	WriteJSON(v interface{}) error
	WriteMessage(msgType int, data []byte) error
	Close() error
}

type Orchestrator struct {
	aisaas   AisaasClient
	splitter *SentenceSplitter
	memory   store.Memory
	opusDec  *opus.Decoder
	opusEnc  OpusEncoder
	log      zerolog.Logger
}

func NewOrchestrator(client AisaasClient, splitter *SentenceSplitter, memory store.Memory, opusDec *opus.Decoder, opusEnc OpusEncoder, log zerolog.Logger) *Orchestrator {
	return &Orchestrator{
		aisaas:   client,
		splitter: splitter,
		memory:   memory,
		opusDec:  opusDec,
		opusEnc:  opusEnc,
		log:      log,
	}
}

// Run orchestrates a single dialogue turn: STT -> Chat (with persona system prompt) -> TTS -> write back to conn.
//
// audioBytes is the raw session segment captured between speech-start and speech-end events
// (the pipeline already buffers + VAD-detects, so we receive the segment directly here).
// Empty audio is a no-op.
func (o *Orchestrator) Run(ctx context.Context, sessionID string, conn Conn, device *aisaas.DeviceInfo, audioBytes []byte) error {
	o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Int("audio_bytes", len(audioBytes)).Msg("orchestrator run start")
	if len(audioBytes) == 0 {
		o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Msg("orchestrator skip: empty audio")
		return nil
	}

	pcm, err := o.opusDec.Decode(audioBytes)
	if err != nil {
		// 损坏的 opus 帧不应让整个对话链路崩溃：仅记录 + 跳过 STT。
		o.log.Warn().Err(err).Int("bytes", len(audioBytes)).Msg("opus decode failed; skipping turn")
		return nil
	}
	o.log.Debug().Str("session", sessionID).Int("pcm_samples", len(pcm)).Msg("opus decoded")

	pcmBytes := int16ToBytes(pcm)
	wavData := wav.PCMToWAV(pcmBytes, 16000, 1, 16)

	o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Int("wav_bytes", len(wavData)).Msg("calling STT")
	sttResult, err := o.aisaas.STT(ctx, "stt-default", wavData)
	if err != nil {
		o.log.Error().Err(err).Str("session", sessionID).Msg("STT failed")
		return fmt.Errorf("stt: %w", err)
	}
	if sttResult.Text == "" {
		o.log.Info().Str("session", sessionID).Msg("STT returned empty text; skipping turn")
		return nil
	}
	o.log.Info().Str("session", sessionID).Str("text", sttResult.Text).Msg("STT ok")

	var messages []aisaas.ChatMessage
	// 1) 注入 persona 的 system prompt（设备绑定的角色）。失败回退为空（demo-chat 自带默认）。
	persona, err := o.aisaas.GetPersonaByDevice(ctx, device.DeviceID, device.UserID)
	if err == nil && persona != nil && persona.SystemPrompt != "" {
		messages = append(messages, aisaas.ChatMessage{Role: "system", Content: persona.SystemPrompt})
		o.log.Info().Str("session", sessionID).Str("persona", persona.Name).Msg("persona system prompt injected")
	} else if err != nil && !errors.Is(err, aisaas.ErrPersonaNotBound) {
		o.log.Warn().Err(err).Str("device_id", device.DeviceID).Msg("get persona by device failed; continuing without system prompt")
	} else if persona != nil {
		o.log.Info().Str("session", sessionID).Str("persona", persona.Name).Msg("persona found but empty system prompt")
	}
	// 2) 历史消息
	memoryMsgs, err := o.memory.GetWindow(ctx, device.DeviceID, 20)
	if err != nil {
		return fmt.Errorf("get window: %w", err)
	}
	for _, m := range memoryMsgs {
		messages = append(messages, aisaas.ChatMessage{Role: m.Role, Content: m.Content})
	}
	// 3) 用户本轮
	messages = append(messages, aisaas.ChatMessage{Role: "user", Content: sttResult.Text})

	// 模型：优先 persona.DefaultModelID（实际 model code），回退到 LLMConfigID。
	model := ""
	if persona != nil && persona.DefaultModelID != "" {
		model = persona.DefaultModelID
	} else if device.LLMConfigID > 0 {
		model = fmt.Sprintf("llm-%d", device.LLMConfigID)
	} else {
		model = "demo-chat"
	}

	stream, err := o.aisaas.Chat(ctx, model, messages)
	if err != nil {
		o.log.Error().Err(err).Str("session", sessionID).Str("model", model).Msg("Chat stream call failed")
		return fmt.Errorf("chat: %w", err)
	}
	defer stream.Close()
	o.log.Info().Str("session", sessionID).Str("model", model).Int("messages", len(messages)).Msg("Chat stream started")

	ttsMsg := protocol.TTSMessage{Type: protocol.TTS, State: protocol.TTSStateStart}
	if err := conn.WriteJSON(ttsMsg); err != nil {
		return err
	}
	o.log.Info().Str("session", sessionID).Msg("TTS start sent to client")

	var fullResponse strings.Builder
	onToken := func(token string) error {
		fullResponse.WriteString(token)
		sentences := o.splitter.Feed(token)
		for _, sent := range sentences {
			if err := o.synthesizeAndSend(ctx, conn, sent, device); err != nil {
				return err
			}
		}
		return nil
	}

	if err := aisaas.ParseLLMStream(stream, onToken); err != nil {
		return fmt.Errorf("llm stream: %w", err)
	}

	if rest := o.splitter.Flush(); rest != "" {
		if err := o.synthesizeAndSend(ctx, conn, rest, device); err != nil {
			return err
		}
	}

	stopMsg := protocol.TTSMessage{Type: protocol.TTS, State: protocol.TTSStateStop}
	if err := conn.WriteJSON(stopMsg); err != nil {
		return err
	}
	o.log.Info().Str("session", sessionID).Msg("TTS stop sent to client")

	if err := o.memory.SaveTurn(ctx, device.DeviceID, sessionID, sttResult.Text, fullResponse.String()); err != nil {
		o.log.Error().Err(err).Msg("save turn failed")
	}
	o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Int("response_chars", fullResponse.Len()).Msg("orchestrator run done")

	return nil
}

func (o *Orchestrator) synthesizeAndSend(ctx context.Context, conn Conn, sentence string, device *aisaas.DeviceInfo) error {
	startMsg := protocol.TTSMessage{Type: protocol.TTS, State: protocol.TTSStateSentenceStart, Text: sentence}
	if err := conn.WriteJSON(startMsg); err != nil {
		return err
	}

	model := fmt.Sprintf("tts-%d", device.TTSConfigID)
	stream, _, err := o.aisaas.TTS(ctx, model, sentence)
	if err != nil {
		o.log.Error().Err(err).Str("model", model).Str("text", sentence).Msg("TTS call failed")
		return fmt.Errorf("tts: %w", err)
	}
	defer stream.Close()
	o.log.Info().Str("model", model).Str("text", sentence).Msg("TTS returned, encoding to opus")

	audioData, err := io.ReadAll(stream)
	if err != nil {
		return fmt.Errorf("read tts stream: %w", err)
	}

	pcm := wav.WAVToPCM16(audioData)
	encoded, err := o.opusEnc.Encode(pcm, 960)
	if err != nil {
		return fmt.Errorf("opus encode: %w", err)
	}

	return conn.WriteMessage(2, encoded)
}

func int16ToBytes(pcm []int16) []byte {
	b := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		b[i*2] = byte(s)
		b[i*2+1] = byte(s >> 8)
	}
	return b
}
