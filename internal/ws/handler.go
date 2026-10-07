package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/opus"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/vad"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/dialogue"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/store"
)

type Handler struct {
	upgrader     websocket.Upgrader
	sm           *SessionManager
	aisaas       aisaas.Authenticator
	deviceInfo   *aisaas.Client
	log          zerolog.Logger
	pipeline     *AudioPipeline
	opusDecoder  *opus.Decoder
	interrupt    InterruptControllerRef
	eventBus     event.EventBusInterface
	orchestrator dialogue.OrchestratorRunner
	// streamOrchestrator：流式 orchestrator（aisaas /dialogue/stream SSE 边收边推）。
	// streamingEnabled && streamOrchestrator != nil 时优先用流式；否则回退到老 orchestrator。
	streamOrchestrator *dialogue.StreamOrchestrator
	streamingEnabled   bool
	deviceStore        store.DeviceStore // 用于在 invokeOrchestrator 时取设备 API Key（Bearer sk-aisaas-...）

	// Per-connection guards to prevent double-fire of dialogue between VAD
	// SpeechEnd and the device's explicit listen-stop message.
	processMu  sync.Mutex
	processing map[string]bool

	// Per-session PCM accumulator (opus 60ms @ 16kHz = 960 samples per frame,
	// but the VAD needs 512-sample chunks). Filled in handleBinaryAudio.
	pcmAccum sync.Map
}

type InterruptControllerRef interface {
	Trigger()
	Triggered() bool
	SetCancel(cancel context.CancelFunc)
}

func (h *Handler) SetPipeline(p *AudioPipeline) {
	h.pipeline = p
}

func (h *Handler) SetInterruptController(ic InterruptControllerRef) {
	h.interrupt = ic
}

func (h *Handler) SetEventBus(bus event.EventBusInterface) {
	h.eventBus = bus
}

// SetOrchestrator installs the dialogue runner. handler.invoke(orchestrator.Run) on listen-stop.
func (h *Handler) SetOrchestrator(o dialogue.OrchestratorRunner) {
	h.orchestrator = o
}

// SetStreamOrchestrator installs the streaming dialogue runner (SSE edge-push).
//
// When enabled=true and streamOrchestrator != nil, invokeOrchestrator prefers the
// streaming path (lower latency). Otherwise it falls back to the legacy
// OrchestratorRunner.
func (h *Handler) SetStreamOrchestrator(so *dialogue.StreamOrchestrator, enabled bool) {
	h.streamOrchestrator = so
	h.streamingEnabled = enabled
}

// SetDeviceClient installs the full aisaas client (we need GetDevice, not just VerifyDeviceToken).
func (h *Handler) SetDeviceClient(c *aisaas.Client) {
	h.deviceInfo = c
}

// SetDeviceStore installs the local device store so the orchestrator can fetch
// the cached aisaas APIKey for Bearer header injection on STT/Chat/TTS calls.
func (h *Handler) SetDeviceStore(s store.DeviceStore) {
	h.deviceStore = s
}

// SetOpusDecoder installs the uplink opus decoder. When set, the handler
// will decode each binary frame into int16 PCM and feed it to the VAD
// pipeline. When unset, the handler falls back to the original raw-frame
// buffering behavior (used by smoke tests).
func (h *Handler) SetOpusDecoder(d *opus.Decoder) {
	h.opusDecoder = d
}

func NewHandler(sm *SessionManager, ac aisaas.Authenticator, log zerolog.Logger) *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
		},
		sm:         sm,
		aisaas:     ac,
		log:        log,
		processing: make(map[string]bool),
	}
}

func (h *Handler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	deviceID := r.Header.Get("Device-Id")
	if deviceID == "" {
		http.Error(w, "missing device-id header", http.StatusBadRequest)
		return
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "missing authorization header", http.StatusUnauthorized)
		return
	}

	if err := h.validateToken(deviceID, token); err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error().Err(err).Msg("upgrade failed")
		return
	}

	sessionID := newSessionID()
	session := NewChatSession(deviceID, sessionID)
	if err := h.sm.Register(session); err != nil {
		h.log.Error().Err(err).Msg("session register failed")
		conn.Close()
		return
	}

	if h.deviceInfo != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if dev, derr := h.deviceInfo.GetDevice(ctx, deviceID); derr == nil {
			session.device = dev
		} else {
			h.log.Warn().Err(derr).Str("device", deviceID).Msg("get device info failed; orchestrator will fall back")
		}
	}

	go h.serveConn(conn, session)
}

func (h *Handler) validateToken(deviceID, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.aisaas.VerifyDeviceToken(ctx, deviceID, token)
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *Handler) serveConn(conn *websocket.Conn, session *ChatSession) {
	defer func() {
		if h.eventBus != nil {
			h.eventBus.Publish(event.NewDeviceDisconnectedEvent(session.DeviceID(), session.ID()))
		}
		h.sm.Remove(session.ID())
		if h.pipeline != nil {
			h.pipeline.ResetSession(session.ID())
		}
		conn.Close()
		session.TransitionTo(StateIdle)
	}()

	if h.eventBus != nil {
		h.eventBus.Publish(event.NewDeviceConnectedEvent(session.DeviceID(), session.ID()))
	}

	if h.pipeline != nil {
		go h.pumpVADEvents(conn, session)
	}

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			h.log.Info().Err(err).Str("device", session.DeviceID()).Msg("read failed")
			return
		}
		session.Touch()

		if msgType == websocket.BinaryMessage {
			h.handleBinaryAudio(session, data)
			continue
		}

		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			h.log.Warn().Err(err).Msg("malformed JSON")
			continue
		}
		switch env.Type {
		case "hello":
			h.handleHello(conn, session, data)
		case "listen":
			h.handleListen(conn, session, data)
		case "abort":
			h.handleAbort(session, data)
		case "ack":
			h.handleAck(session, data)
		case "mcp", "iot":
			h.log.Info().Str("type", env.Type).Msg("received; ack-only in MVP")
		default:
			h.log.Warn().Str("type", env.Type).Msg("unknown message type")
		}
	}
}

// handleBinaryAudio decodes one uplink opus frame into int16 PCM and:
//  1. feeds the PCM (in 512-sample chunks) to the VAD pipeline so the Silero
//     model sees continuous audio + gets to drive its state machine, and
//  2. once VAD is in the SpeechContinue state, accumulates the same PCM into
//     the session's audio buffer for downstream STT.
//
// This mirrors xiaozhi-java's VadService.processAudio path: pre-speech frames
// are seen by VAD (so it can detect SpeechStart) but NOT captured for STT,
// which avoids sending silence to the speech recogniser.
func (h *Handler) handleBinaryAudio(session *ChatSession, opusFrame []byte) {
	h.log.Debug().Str("device", session.DeviceID()).Int("bytes", len(opusFrame)).Msg("audio frame received")

	if h.opusDecoder == nil || h.pipeline == nil {
		// Smoke-test path: no decoder, just buffer raw bytes verbatim.
		session.AudioBuffer().Write(opusFrame)
		return
	}

	pcm, err := h.opusDecoder.Decode(opusFrame)
	if err != nil {
		h.log.Warn().Err(err).Str("device", session.DeviceID()).Msg("opus decode failed; dropping frame")
		return
	}
	if len(pcm) == 0 {
		return
	}

	// Push into per-session PCM accumulator; pull out 512-sample chunks and
	// feed them to the VAD pipeline. Without chunking the model sees
	// discontinuous chunks (opus 60 ms @ 16 kHz = 960 samples per frame, but
	// Silero needs 512-sample windows) and never converges.
	v, _ := h.pcmAccum.LoadOrStore(session.ID(), &pcmAccumulator{})
	acc := v.(*pcmAccumulator)
	acc.push(pcm)
chunks := 0
		for {
			chunk := acc.take(512)
			if chunk == nil {
				break
			}
			// Diagnostic: per chunk, dump PCM peak/avg so we can tell whether
			// the uplink audio actually has speech content or is just zeros.
			peak := int16(0)
			var sum int64
			for _, s := range chunk {
				if s < 0 {
					if -s > peak {
						peak = -s
					}
				} else if s > peak {
					peak = s
				}
				sum += int64(s)
			}
			h.log.Debug().Str("device", session.DeviceID()).Int("peak", int(peak)).Int64("sum", sum).Int("chunk_idx", chunks).Msg("vad PCM chunk stats")
			// Dump raw int16 PCM to wav so we can audit what the device actually sent.
			if session.dumpFile != nil {
				session.dumpFile.WriteChunk(chunk)
			}
			h.pipeline.Feed(session.ID(), chunk)
			if h.pipeline.IsSpeaking(session.ID()) {
				session.AudioBuffer().WritePCM(chunk)
			}
			chunks++
		}
	if chunks > 0 {
		h.log.Debug().Str("device", session.DeviceID()).Int("pcm_samples", len(pcm)).Int("vad_chunks", chunks).Msg("vad chunks fed")
	}
}

func (h *Handler) handleHello(conn *websocket.Conn, session *ChatSession, raw []byte) {
	var msg protocol.HelloMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		h.log.Warn().Err(err).Msg("hello parse failed")
		return
	}
	h.log.Info().
		Str("device", session.DeviceID()).
		Int("client_version", msg.Version).
		Interface("client_audio_params", msg.AudioParams).
		Interface("client_features", msg.Features).
		Msg("hello received (full)")
	resp := protocol.NewServerHello(session.ID(), protocol.AudioParams{
		Format: "opus", SampleRate: 16000, Channels: 1, FrameDuration: 60,
	})
	if err := conn.WriteJSON(resp); err != nil {
		h.log.Error().Err(err).Msg("hello response failed")
	}
	h.log.Info().Str("device", session.DeviceID()).Msg("hello exchanged")

	// 保活：xz-server 重启后 InMemoryDeviceStore 清空。设备下次 hello 时若本地无 APIKey，
	// 主动调 aisaas RegisterDevice 拿到新 key（已开户走 mintDeviceAPIKey → 新 key 签发，旧 key 失效但无害）。
	// 这样后续 VAD 触发的 STT/LLM/TTS 调用能注入 Authorization: Bearer sk-aisaas-...。
	if h.deviceStore != nil && h.deviceInfo != nil {
		go func(deviceID string) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			localDev, _ := h.deviceStore.GetDevice(ctx, deviceID)
			if localDev != nil && localDev.APIKey != "" {
				return
			}
			regResp, regErr := h.deviceInfo.RegisterDevice(ctx, deviceID, deviceID, "esp32", "")
			if regErr != nil || regResp == nil {
				h.log.Warn().Err(regErr).Str("device", deviceID).Msg("hello-time RegisterDevice failed (non-fatal)")
				return
			}
			token := ""
			if localDev != nil {
				token = localDev.Token
			}
			if token == "" {
				b := make([]byte, 16)
				_, _ = rand.Read(b)
				token = hex.EncodeToString(b)
			}
			if upsertErr := h.deviceStore.UpsertDevice(ctx, &store.Device{
				DeviceID: deviceID,
				Token:    token,
				APIKey:   regResp.APIKey,
			}); upsertErr != nil {
				h.log.Warn().Err(upsertErr).Str("device", deviceID).Msg("hello-time UpsertDevice failed (non-fatal)")
				return
			}
			h.log.Info().Str("device", deviceID).Int64("tenant", regResp.TenantID).Msg("APIKey warmed via hello RegisterDevice")
		}(session.DeviceID())
	}
}

func (h *Handler) handleListen(conn *websocket.Conn, session *ChatSession, raw []byte) {
	var msg protocol.ListenMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		h.log.Warn().Err(err).Str("device", session.DeviceID()).Msg("listen parse failed")
		return
	}
	switch msg.State {
	case protocol.ListenStateStart:
		h.log.Info().Str("device", session.DeviceID()).Str("session", session.ID()).Msg("listen-start received")
		// If a prior TTS/orchestrator is still running, abort it.
		if h.interrupt != nil {
			h.interrupt.Trigger()
		}
		_ = session.TransitionTo(StateListening)
	case protocol.ListenStateStop:
		pcmBytes := session.AudioBuffer().Len()
		h.log.Info().Str("device", session.DeviceID()).Str("session", session.ID()).Int("pcm_bytes", pcmBytes).Msg("listen-stop received → triggering orchestrator")
		h.tryInvokeOrchestrator(conn, session, "listen-stop")
		_ = session.TransitionTo(StateIdle)
	case protocol.ListenStateDetect:
		h.log.Debug().Str("device", session.DeviceID()).Msg("listen-detect (auto Wake Up)")
	}
}

// tryInvokeOrchestrator is a guarded entry point used by both the VAD
// SpeechEnd handler and the listen-stop handler. Only one of the two paths
// ever wins; the other sees an empty audio buffer and returns silently.
func (h *Handler) tryInvokeOrchestrator(conn *websocket.Conn, session *ChatSession, source string) {
	h.processMu.Lock()
	if h.processing[session.ID()] {
		h.processMu.Unlock()
		h.log.Debug().Str("session", session.ID()).Str("source", source).Msg("orchestrator already running; skipping duplicate trigger")
		return
	}
	h.processing[session.ID()] = true
	h.processMu.Unlock()

	defer func() {
		h.processMu.Lock()
		delete(h.processing, session.ID())
		h.processMu.Unlock()
	}()

	if err := h.invokeOrchestrator(conn, session); err != nil {
		h.log.Error().Err(err).Str("device", session.DeviceID()).Msg("orchestrator run failed")
	}
}

// invokeOrchestrator drains the session audio buffer (raw int16 PCM little-
// endian bytes, already decoded from uplink opus) and runs the full
// STT → Chat → TTS pipeline. Returns silently when there is no audio to
// process (e.g. VAD fired but the device's wake-up produced no real speech).
func (h *Handler) invokeOrchestrator(conn *websocket.Conn, session *ChatSession) error {
	if h.orchestrator == nil {
		return nil
	}
	device := session.Device()
	if device == nil {
		device = &aisaas.DeviceInfo{DeviceID: session.DeviceID()}
	}
	// 从本地 deviceStore 注入 APIKey，供 orchestrator 调 STT/Chat/TTS 时附加 Bearer。
	if h.deviceStore != nil && device.APIKey == "" {
		if localDev, err := h.deviceStore.GetDevice(context.Background(), session.DeviceID()); err == nil && localDev != nil {
			device.APIKey = localDev.APIKey
		}
	}
	// store 也没 → 同步兜底调 aisaas RegisterDevice 拿 key（不阻塞 ≤3s），
	// 避免「设备 hello 之后到 listen-stop 之间没人调 RegisterDevice」导致 STT 401。
	if device.APIKey == "" && h.deviceInfo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		regResp, regErr := h.deviceInfo.RegisterDevice(ctx, session.DeviceID(), session.DeviceID(), "esp32", "")
		cancel()
		if regErr == nil && regResp != nil {
			device.APIKey = regResp.APIKey
			if h.deviceStore != nil {
				localDev, _ := h.deviceStore.GetDevice(context.Background(), session.DeviceID())
				token := ""
				if localDev != nil {
					token = localDev.Token
				}
				if token == "" {
					b := make([]byte, 16)
					_, _ = rand.Read(b)
					token = hex.EncodeToString(b)
				}
				_ = h.deviceStore.UpsertDevice(context.Background(), &store.Device{
					DeviceID: session.DeviceID(),
					Token:    token,
					APIKey:   regResp.APIKey,
				})
			}
			h.log.Info().Str("device", session.DeviceID()).Int64("tenant", regResp.TenantID).Msg("APIKey warmed via invokeOrchestrator fallback")
		}
	}
	if device.APIKey == "" {
		h.log.Warn().Str("device", session.DeviceID()).Msg("device APIKey missing; STT/LLM/TTS calls will return 401")
	}
	pcmBytes := session.AudioBuffer().Drain()
	if len(pcmBytes) == 0 {
		return nil
	}
	_ = session.TransitionTo(StateThinking)
	defer func() { _ = session.TransitionTo(StateIdle) }()

	// Reset VAD session state so the next turn starts with fresh context.
	if h.pipeline != nil {
		h.pipeline.ResetSession(session.ID())
	}

	// Interruptible context: when the device sends a new listen-start (or
	// VAD SpeechStart) mid-turn, the interrupt cancels STT/Chat/TTS so we
	// can answer faster.
	//
	// Timeout must comfortably exceed aisaas dialogue.run + frame pacing.
	// Typical Chinese reply: ~20 s dialogue + ~50 s opus frames @ 60 ms/frame
	// (Java ScheduledPlayer.sendSpeechWithBurstMode). 120 s gives 2× headroom.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if h.interrupt != nil {
		h.interrupt.SetCancel(cancel)
	}

	// Streaming path preferred when configured; on failure logs + sends an
// error JSON to the client so the device can decide to re-listen. We do NOT
// fall back to the legacy orchestrator: pcmBytes was already drained from
// the session buffer by this point, so re-running with the legacy path
// would produce silent output (legacy STT would fail on empty audio).
	if h.streamingEnabled && h.streamOrchestrator != nil {
		// protocolVersion from session hello handshake (default "1").
		protocolVersion := "1"
		apiKey := device.APIKey
		if err := h.streamOrchestrator.Run(ctx, session.DeviceID(), pcmBytes, conn, protocolVersion, 16000, apiKey); err != nil {
			h.log.Error().Err(err).Str("device", session.DeviceID()).Msg("stream orchestrator failed")
		}
		return nil
	}
	return h.orchestrator.Run(ctx, session.ID(), conn, device, pcmBytes)
}

func (h *Handler) handleAbort(session *ChatSession, raw []byte) {
	var msg protocol.AbortMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	session.AudioBuffer().Drain()
	if h.pipeline != nil {
		h.pipeline.ResetSession(session.ID())
	}
	_ = session.TransitionTo(StateIdle)
	h.log.Info().Str("reason", msg.Reason).Msg("abort received")
}

func (h *Handler) handleAck(session *ChatSession, raw []byte) {
	var msg protocol.AckMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	h.log.Debug().Str("msgType", msg.MsgType).Str("status", msg.Status).Msg("ack received")
}

// pcmAccumulator holds decoded int16 PCM samples waiting to be handed to the
// VAD pipeline in 512-sample chunks. Per-session (keyed by sessionID).
type pcmAccumulator struct {
	mu    sync.Mutex
	queue []int16
}

func (a *pcmAccumulator) push(samples []int16) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.queue = append(a.queue, samples...)
}

func (a *pcmAccumulator) take(n int) []int16 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.queue) < n {
		return nil
	}
	out := make([]int16, n)
	copy(out, a.queue[:n])
	a.queue = a.queue[n:]
	return out
}

func (a *pcmAccumulator) discard() {
	a.mu.Lock()
	a.queue = nil
	a.mu.Unlock()
}

// pumpVADEvents drains the pipeline's event channel for the lifetime of
// this connection. On SpeechEnd for this session, drain the audio buffer
// and trigger the orchestrator (mirrors the listen-stop path).
func (h *Handler) pumpVADEvents(conn *websocket.Conn, session *ChatSession) {
	events := h.pipeline.Events()
	for evt := range events {
		if evt.SessionID != session.ID() {
			continue
		}
		switch evt.Status {
		case vad.SpeechStart:
			h.log.Info().Str("device", session.DeviceID()).Str("session", session.ID()).Msg("VAD speech start → trigger interrupt")
			if h.interrupt != nil {
				h.interrupt.Trigger()
			}
		case vad.SpeechEnd:
			pcmBytes := session.AudioBuffer().Len()
			h.log.Info().Str("device", session.DeviceID()).Str("session", session.ID()).Int("pcm_bytes", pcmBytes).Msg("VAD speech end → triggering orchestrator")
			h.tryInvokeOrchestrator(conn, session, "vad-end")
		}
	}
}