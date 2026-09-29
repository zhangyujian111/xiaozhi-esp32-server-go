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
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/ws"
)

var ErrNotImplemented = errors.New("not implemented")

type AisaasClient interface {
	STT(ctx context.Context, model string, wavData []byte) (*aisaas.STTResult, error)
	Chat(ctx context.Context, model string, messages []aisaas.ChatMessage) (io.ReadCloser, error)
	TTS(ctx context.Context, model, text string) (io.ReadCloser, string, error)
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

func (o *Orchestrator) Run(ctx context.Context, session *ws.ChatSession, conn Conn, device *aisaas.DeviceInfo) error {
	audioData := session.AudioBuffer().Drain()
	if len(audioData) == 0 {
		return nil
	}

	pcm, err := o.opusDec.Decode(audioData)
	if err != nil {
		return fmt.Errorf("opus decode: %w", err)
	}

	pcmBytes := int16ToBytes(pcm)
	wavData := wav.PCMToWAV(pcmBytes, 16000, 1, 16)

	sttResult, err := o.aisaas.STT(ctx, "stt-default", wavData)
	if err != nil {
		return fmt.Errorf("stt: %w", err)
	}

	model := fmt.Sprintf("llm-%d", device.LLMConfigID)
	memoryMsgs, err := o.memory.GetWindow(ctx, device.DeviceID, 20)
	if err != nil {
		return fmt.Errorf("get window: %w", err)
	}

	var messages []aisaas.ChatMessage
	for _, m := range memoryMsgs {
		messages = append(messages, aisaas.ChatMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, aisaas.ChatMessage{Role: "user", Content: sttResult.Text})

	stream, err := o.aisaas.Chat(ctx, model, messages)
	if err != nil {
		return fmt.Errorf("chat: %w", err)
	}
	defer stream.Close()

	ttsMsg := protocol.TTSMessage{Type: protocol.TTS, State: protocol.TTSStateStart}
	if err := conn.WriteJSON(ttsMsg); err != nil {
		return err
	}

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

	if err := o.memory.SaveTurn(ctx, device.DeviceID, session.ID(), sttResult.Text, fullResponse.String()); err != nil {
		o.log.Error().Err(err).Msg("save turn failed")
	}

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
		return fmt.Errorf("tts: %w", err)
	}
	defer stream.Close()

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
