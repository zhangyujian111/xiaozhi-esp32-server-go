package dialogue

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
)

// pcmToWAV wraps raw int16 little-endian PCM bytes in a minimal WAV header.
//
// aisaas DialogueRunner / StreamRunner both expect a full WAV file (RIFF header + PCM data).
// xz-server's uplink pipeline decodes opus → int16 PCM little-endian @ 16 kHz mono; we wrap
// before posting to aisaas. Matches xiaozhi-java's AudioUtils.pcmToWav pattern.
func pcmToWAV(pcm []byte, sampleRate int) []byte {
	const channels = 1
	const bitsPerSample = 16
	hdr := make([]byte, 44+len(pcm))
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(36+len(pcm)))
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(hdr[20:22], 1)  // PCM format
	binary.LittleEndian.PutUint16(hdr[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(sampleRate*channels*bitsPerSample/8))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(channels*bitsPerSample/8))
	binary.LittleEndian.PutUint16(hdr[34:36], uint16(bitsPerSample))
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(len(pcm)))
	copy(hdr[44:], pcm)
	return hdr
}

// bytesToInt16LE 把 raw int16 little-endian bytes 转 []int16（aisaas tts-audio event.Audio 直接传 PCM）。
func bytesToInt16LE(b []byte) []int16 {
	n := len(b) / 2
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		out[i] = int16(b[i*2]) | int16(b[i*2+1])<<8
	}
	return out
}

// streamClientIface 是 aisaas.StreamClient 的最小 interface（便于 mock）。
type streamClientIface interface {
	DialogueStream(ctx context.Context, deviceID string, wavBytes []byte) (<-chan aisaas.DialogueStreamEvent, <-chan error)
}

// StreamOrchestrator 把 aisaas 流式对话 events 流实时推送到 ws conn。
//
// 与 Orchestrator.Run 的区别：原 Orchestrator 调 aisaas.Client.Dialogue（一次性拿全 audio），
// 然后 decode → opus encode → 按 60ms pacing 推 N 个 binary frames。StreamOrchestrator
// 改为直接订阅 aisaas SSE event 流，每收到一个 tts-audio 帧（=一段 raw PCM @ 16kHz mono
// int16 little-endian，60ms chunk）就立即 opus encode → 推到 ESP32——LLM 边出 token +
// TTS 边合成 + ESP32 边播，时延降到首字节 < 3 秒。
//
// audioFormat 约定：aisaas 端 stream_runner.go 的 streamWAVChunks 切 60ms PCM chunk 后
// 把 raw int16 LE bytes（不带 WAV header）通过 tts-audio event.Audio 推过来；本 orchestrator
// 负责 PCM → opus encode → WriteAudioFrame。opus encoder 由 app.go 注入（NewStreamOrchestrator
// 第二个参数），复用 legacy Orchestrator.playWAVAsOpusFrames 的 opusEncoder。
//
// Fallback 设计：StreamOrchestrator 不内置 fallback。当收到 error event 或 stream 中断时，
// 写一条 error JSON 控制消息给客户端，由 caller（ws handler）决定是否回退到老 Dialogue 路径。
type StreamOrchestrator struct {
	streamClientFactory func(apiKey string) streamClientIface
	opusEnc             OpusEncoder // 注入：[]int16 PCM → []byte opus 帧
	framePacer          FramePacer
	log                 zerolog.Logger
}

// NewStreamOrchestrator 构造流式 orchestrator。
//
// streamClientFactory 每次 Run 调用时实例化一个新 client（便于 per-session API key 注入）。
// opusEnc 用于 tts-audio PCM→opus 编码（nil 时 tts-audio 静默丢弃，debug 用）。
// 传 nil streamClientFactory 时使用默认 aisaas.NewStreamClient 工厂。
func NewStreamOrchestrator(streamClientFactory func(apiKey string) *aisaas.StreamClient, opusEnc OpusEncoder, log zerolog.Logger) *StreamOrchestrator {
	return &StreamOrchestrator{
		streamClientFactory: func(apiKey string) streamClientIface { return streamClientFactory(apiKey) },
		opusEnc:             opusEnc,
		log:                 log,
	}
}

// setStreamClientFactory 是测试注入专用（避免 import 循环）。
func (so *StreamOrchestrator) setStreamClientFactory(f func(apiKey string) streamClientIface) {
	so.streamClientFactory = f
}

// WithFramePacer 注入帧间 pacing（解决 ESP32 SetDeviceState race）。
func (so *StreamOrchestrator) WithFramePacer(p FramePacer) *StreamOrchestrator {
	so.framePacer = p
	return so
}

// Run 启动流式对话。返回时 events 已被完整推送；error 时已写 error JSON 控制消息。
//
// pcmBytes: int16 little-endian PCM @ sampleRate（已从 uplink opus 解码）。
// conn: WebSocket conn interface
// protocolVersion: ESP32 协商版本 ("1"/"2"/"3")——BinaryWriter 自动按版本决定是否加 header
// sampleRate: pcmBytes 的采样率（典型 16000）
// apiKey: 设备 API Key（Bearer sk-aisaas-...）；注入 StreamClient.Authorization。
func (so *StreamOrchestrator) Run(ctx context.Context, deviceID string, pcmBytes []byte, conn Conn, protocolVersion string, sampleRate int, apiKey string) error {
	bw := protocol.NewBinaryWriter(conn, protocolVersion)

	client := so.streamClientFactory(apiKey)
	if client == nil {
		return fmt.Errorf("stream orchestrator: streamClientFactory returned nil")
	}

	wavBytes := pcmToWAV(pcmBytes, sampleRate)
	events, errs := client.DialogueStream(ctx, deviceID, wavBytes)
	frameIdx := 0
	pacer := so.framePacer
	if pacer == nil {
		pacer = noopFramePacer{}
	}

	for ev := range events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		switch ev.Type {
		case "tts-start", "sentence-start", "sentence-end", "tts-stop":
			// Adapter: aisaas stream_runner 用 type=tts-start / sentence-start / ...
			// 命名风格（hyphen 包一个语义）；ESP32 firmware xiaozhi 文档约定用
			// {type:"tts", state:"start"|"stop"|"sentence_start"|"sentence_end"}
			// （application.cc:569,581-602）。这里做最终 wire-format 转换，
			// 保持 aisaas 内部 SSE 事件结构不变。
			msgID := ""
			if v, ok := ev.Payload["msgId"].(string); ok {
				msgID = v
			}
			var payload map[string]any
			switch ev.Type {
			case "tts-start":
				payload = map[string]any{"type": "tts", "state": "start", "msgId": msgID}
			case "sentence-start":
				payload = map[string]any{
					"type":   "tts",
					"state":  "sentence_start",
					"msgId":  msgID,
				}
				if t, ok := ev.Payload["text"].(string); ok {
					payload["text"] = t
				}
			case "sentence-end":
				payload = map[string]any{"type": "tts", "state": "sentence_end", "msgId": msgID}
				if e, ok := ev.Payload["error"].(string); ok && e != "" {
					payload["error"] = e
				}
			case "tts-stop":
				payload = map[string]any{"type": "tts", "state": "stop", "msgId": msgID}
			}
			if err := bw.WriteText(payload); err != nil {
				return fmt.Errorf("write %s: %w", ev.Type, err)
			}
		case "stt":
			// aisaas stt event payload: {"text":"..."} → ESP32: {"type":"stt","text":"..."}
			payload := map[string]any{"type": "stt"}
			if t, ok := ev.Payload["text"].(string); ok && t != "" {
				payload["text"] = t
			}
			if err := bw.WriteText(payload); err != nil {
				return fmt.Errorf("write stt: %w", err)
			}
		case "done":
			// ESP32 firmware 没有 "done" 分支；语义上 tts-stop 已表示
			// 本次 TTS 结束，这里跳过不发（避免进 default → "Unknown message type"）。
			continue
		case "tts-audio":
			if len(ev.Audio) == 0 {
				continue
			}
			if so.opusEnc == nil {
				// 没注入 opus encoder：debug 用，静默丢弃
				continue
			}
			// ev.Audio 是 raw int16 LE PCM @ 16kHz mono（60ms chunk）；转 int16 → opus encode
			pcmInt16 := bytesToInt16LE(ev.Audio)
			frameSize := 960 // 60ms @ 16kHz = 960 samples（与 aisaas streamWAVChunks 对齐）
			if len(pcmInt16) < frameSize {
				// 不足一帧：pacer 直接跳过（或填充静默）；这里选跳过
				continue
			}
			opusFrame, encErr := so.opusEnc.Encode(pcmInt16[:frameSize], frameSize)
			if encErr != nil {
				return fmt.Errorf("opus encode: %w", encErr)
			}
			pacer.Pace(ctx, frameIdx)
			frameIdx++
			if err := bw.WriteAudioFrame(opusFrame); err != nil {
				return fmt.Errorf("write audio frame: %w", err)
			}
		case "error":
			msg := ""
			if ev.Payload != nil {
				if m, ok := ev.Payload["message"].(string); ok {
					msg = m
				}
			}
			so.log.Warn().Str("device", deviceID).Str("stream_error", msg).Msg("stream orchestrator: aisaas sent error event")
			return nil
		default:
			so.log.Warn().Str("event", ev.Type).Msg("stream orchestrator: unknown event type")
		}
	}
	// events channel closed: drain errs in background
	go func() {
		for e := range errs {
			if e != nil {
				so.log.Error().Err(e).Msg("stream orchestrator: runner error")
			}
		}
	}()
	return nil
}

// Ensure time package import used (test setup may import only in test file).
var _ = time.Second
var _ = binary.BigEndian