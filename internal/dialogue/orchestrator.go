package dialogue

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type AisaasClient interface {
	Dialogue(ctx context.Context, deviceID string, wavBytes []byte) (*aisaas.DialogueResult, error)
	GetPersonaByDevice(ctx context.Context, deviceID string, tenantID int64) (*aisaas.Persona, error)
}

type OpusEncoder interface {
	Encode(pcm []int16, frameSize int) ([]byte, error)
}

// FramePacer inserts a delay between consecutive opus frames sent to ESP32.
//
// Why pacing is needed: xz-go sends a TTS-start JSON message immediately
// followed by N opus frames. ESP32's websocket layer receives all of them
// into its internal buffer, then dispatches them sequentially. The TTS-start
// handler schedules SetDeviceState(Speaking) onto the main thread via
// Schedule() — by the time the main thread runs that lambda, the websocket
// loop has already dispatched the first opus frame, which sees
// device_state == listening and drops the packet. The result is silent
// playback even though frames arrive.
//
// Mirrors xiaozhi-java's ScheduledPlayer.sendSpeechWithBurstMode: a small
// inter-frame delay (default 60 ms, matching the opus frame duration) gives
// ESP32's main thread time to flip device_state to Speaking before the next
// frame arrives.
type FramePacer interface {
	Pace(ctx context.Context, frameIdx int)
}

// noopFramePacer is the default — used when pacing is disabled (tests, smoke
// runs). Sends frames back-to-back without delay.
type noopFramePacer struct{}

func (noopFramePacer) Pace(_ context.Context, _ int) {}

// intervalFramePacer sleeps `interval` between consecutive frames.
// Frame index 0 is sent immediately as a prebuffer (so the first opus frame
// reaches ESP32 ~in parallel with the TTS-start JSON, and the second frame
// arrives after the device_state transition has settled). Frames 1..N-1 wait
// `interval`. Honours ctx cancellation.
type intervalFramePacer struct {
	interval time.Duration
}

func newIntervalFramePacer(interval time.Duration) *intervalFramePacer {
	return &intervalFramePacer{interval: interval}
}

// NewFramePacer creates a FramePacer that sleeps `interval` between consecutive
// opus frames (frame index 0 is sent immediately as a prebuffer). Honours ctx
// cancellation. Typical value: 60ms (matches a single 60ms opus frame).
func NewFramePacer(interval time.Duration) FramePacer {
	return newIntervalFramePacer(interval)
}

func (p *intervalFramePacer) Pace(ctx context.Context, frameIdx int) {
	if frameIdx == 0 {
		return // prebuffer: first frame rides alongside TTS-start JSON
	}
	timer := time.NewTimer(p.interval)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

type Conn interface {
	WriteJSON(v interface{}) error
	WriteMessage(msgType int, data []byte) error
	Close() error
}

type Orchestrator struct {
	aisaas     AisaasClient
	splitter   *SentenceSplitter
	memory     store.Memory
	opusEnc    OpusEncoder
	framePacer FramePacer
	log        zerolog.Logger
}

func NewOrchestrator(client AisaasClient, splitter *SentenceSplitter, memory store.Memory, opusEnc OpusEncoder, log zerolog.Logger) *Orchestrator {
	return &Orchestrator{
		aisaas:     client,
		splitter:   splitter,
		memory:     memory,
		opusEnc:    opusEnc,
		framePacer: noopFramePacer{},
		log:        log,
	}
}

// WithFramePacer installs a custom frame pacer (returns receiver for chaining).
// Used by tests and by production wiring that wants real pacing.
func (o *Orchestrator) WithFramePacer(p FramePacer) *Orchestrator {
	o.framePacer = p
	return o
}

// Run orchestrates a single dialogue turn via aisaas POST /internal/api/v1/dialogue/run.
// It replaces the old STT -> Chat -> TTS trio with a single API call.
//
// pcmBytes is raw int16 little-endian PCM (16 kHz mono) captured between
// SpeechStart and SpeechEnd. Empty PCM is a no-op.
//
// aisaas internally handles device -> persona binding -> model registry
// -> STT -> LLM -> TTS, returning transcribed text + opus audio.
func (o *Orchestrator) Run(ctx context.Context, sessionID string, conn Conn, device *aisaas.DeviceInfo, pcmBytes []byte) error {
	// Skip empty audio
	if len(pcmBytes) == 0 {
		o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Msg("orchestrator skip: empty audio")
		return nil
	}

	// Wrap PCM in WAV header (16 kHz mono 16-bit)
	wavData := wav.PCMToWAV(pcmBytes, 16000, 1, 16)
	o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Int("wav_bytes", len(wavData)).Msg("calling dialogue")

	// Single dialogue API call — aisaas handles STT/LLM/TTS internally
	result, err := o.aisaas.Dialogue(ctx, device.DeviceID, wavData)
	if err != nil {
		o.log.Error().Err(err).Str("session", sessionID).Msg("dialogue failed")
		return fmt.Errorf("dialogue: %w", err)
	}

	o.log.Info().
		Str("session", sessionID).
		Str("userText", result.UserText).
		Str("replyText", result.ReplyText).
		Msg("dialogue returned")

	// Send TTS start
	ttsStart := protocol.TTSMessage{Type: protocol.TTS, State: protocol.TTSStateStart}
	if err := conn.WriteJSON(ttsStart); err != nil {
		return fmt.Errorf("send tts start: %w", err)
	}

	// Send audio directly — aisaas currently returns wav; xiaozhi-go decodes to PCM
	// then encodes to opus frames for ESP32. ESP32 only plays back raw opus frames.
	if err := o.playAudioToClient(ctx, sessionID, conn, result.Audio, result.SampleRate, result.AudioFormat); err != nil {
		return err
	}

	// Send TTS stop
	ttsStop := protocol.TTSMessage{Type: protocol.TTS, State: protocol.TTSStateStop}
	if err := conn.WriteJSON(ttsStop); err != nil {
		return fmt.Errorf("send tts stop: %w", err)
	}

	// Save turn to memory
	if err := o.memory.SaveTurn(ctx, device.DeviceID, sessionID, result.UserText, result.ReplyText); err != nil {
		o.log.Error().Err(err).Msg("save turn failed")
	}

	o.log.Info().Str("session", sessionID).Str("device", device.DeviceID).Msg("orchestrator run done")
	return nil
}

// playAudioToClient forwards audio to the WebSocket client.
//
// aisaas currently returns audio in two formats (controlled by audioFormat field):
//   - "wav" (current default): full WAV file bytes. We decode → PCM int16 →
//     opus.Encode per 60ms frame → each frame sent as a separate binary WS message.
//   - "opus": raw pre-encoded opus frames. Forwarded as a single binary WS message.
//
// Historical bug: the original playOpusToClient assumed aisaas already returned
// opus frames and forwarded the bytes directly via conn.WriteMessage(2, audio).
// Since aisaas actually returns WAV bytes, ESP32 tried to decode them as raw
// opus frames and produced no sound (silent playback).
func (o *Orchestrator) playAudioToClient(ctx context.Context, sessionID string, conn Conn, audio []byte, sampleRate int, audioFormat string) error {
	if len(audio) == 0 {
		o.log.Info().Str("session", sessionID).Msg("no audio to play")
		return nil
	}

	switch audioFormat {
	case "", "wav":
		return o.playWAVAsOpusFrames(ctx, sessionID, conn, audio, sampleRate)
	case "opus":
		o.log.Debug().Str("session", sessionID).Int("audio_bytes", len(audio)).Msg("sending opus audio to client (raw)")
		return conn.WriteMessage(2, audio)
	default:
		return fmt.Errorf("unsupported audioFormat=%q (expected wav|opus)", audioFormat)
	}
}

// playWAVAsOpusFrames decodes WAV bytes → PCM int16 → opus.Encode 60ms frames →
// writes each frame as a separate binary WS message.
func (o *Orchestrator) playWAVAsOpusFrames(ctx context.Context, sessionID string, conn Conn, audio []byte, sampleRate int) error {
	pcm, sr, ch, bps, err := wav.WAVToPCM(audio)
	if err != nil {
		return fmt.Errorf("decode wav: %w", err)
	}
	if bps != 16 {
		return fmt.Errorf("unsupported wav bitsPerSample=%d (only 16 supported)", bps)
	}
	if ch != 1 {
		return fmt.Errorf("unsupported wav channels=%d (only mono supported)", ch)
	}

	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[2*i : 2*i+2]))
	}

	frameSize := sr * 60 / 1000 // 60ms frames (1440 @ 24kHz, 960 @ 16kHz)
	if frameSize <= 0 {
		return fmt.Errorf("invalid sampleRate=%d for opus frame calculation", sr)
	}

	frames := 0
	totalBytes := 0
	pacerName := "<nil>"
	if o.framePacer != nil {
		pacerName = fmt.Sprintf("%T", o.framePacer)
	}
	o.log.Info().Str("session", sessionID).Str("pacer", pacerName).Msg("pacing config")
	// 完整帧
	for i := 0; i+frameSize <= len(samples); i += frameSize {
		if o.framePacer != nil {
			o.framePacer.Pace(ctx, frames)
		}
		if err := o.encodeAndWriteOpusFrame(conn, samples[i:i+frameSize], frameSize); err != nil {
			return err
		}
		frames++
		totalBytes += 0 // actual bytes tracked inside encodeAndWriteOpusFrame
	}
	// 不足一帧的尾部用 0 padding（避免丢音）
	remainder := len(samples) % frameSize
	if remainder > 0 {
		padded := make([]int16, frameSize)
		copy(padded, samples[len(samples)-remainder:])
		if o.framePacer != nil {
			o.framePacer.Pace(ctx, frames)
		}
		if err := o.encodeAndWriteOpusFrame(conn, padded, frameSize); err != nil {
			return err
		}
		frames++
	}

	o.log.Info().
		Str("session", sessionID).
		Int("frames", frames).
		Int("pcm_samples", len(samples)).
		Int("sample_rate", sr).
		Msg("sent opus frames (decoded from wav)")
	return nil
}

func (o *Orchestrator) encodeAndWriteOpusFrame(conn Conn, samples []int16, frameSize int) error {
	frame, err := o.opusEnc.Encode(samples, frameSize)
	if err != nil {
		return fmt.Errorf("opus encode: %w", err)
	}
	return conn.WriteMessage(2, frame)
}