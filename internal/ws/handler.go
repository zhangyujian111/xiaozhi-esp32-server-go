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
)

type Handler struct {
	upgrader      websocket.Upgrader
	sm            *SessionManager
	aisaas        aisaas.Authenticator
	deviceInfo    *aisaas.Client
	log           zerolog.Logger
	pipeline      *AudioPipeline
	opusDecoder   *opus.Decoder
	interrupt     InterruptControllerRef
	eventBus      event.EventBusInterface
	orchestrator  dialogue.OrchestratorRunner
	pcmAccum      sync.Map // sessionID → *pcmAccumulator (decoded PCM waiting for 512-sample VAD frames)
}

type InterruptControllerRef interface {
	Trigger()
	Triggered() bool
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

// SetDeviceClient installs the full aisaas client (we need GetDevice, not just VerifyDeviceToken).
func (h *Handler) SetDeviceClient(c *aisaas.Client) {
	h.deviceInfo = c
}

// SetOpusDecoder installs the uplink opus decoder. When set, the handler
// will decode each binary frame into int16 PCM and feed it to the VAD
// pipeline (chunks of 512 samples). When unset, the handler falls back to
// the original raw-frame buffering behavior (used by smoke tests).
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
		sm:     sm,
		aisaas: ac,
		log:    log,
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

	// 拉取设备详情（含 tenantId，供 persona 路由 / 模型 / TTS 配置使用）。
	// 失败不致命：orchestrator 会 fallback 到 persona 默认模型。
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
		conn.Close()
		session.TransitionTo(StateIdle)
	}()

	if h.eventBus != nil {
		h.eventBus.Publish(event.NewDeviceConnectedEvent(session.DeviceID(), session.ID()))
	}

	// Pump VAD events: when the pipeline emits SpeechEnd for THIS session,
	// trigger the orchestrator. Other sessions' events are ignored.
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
			session.AudioBuffer().Write(data)
			h.log.Debug().Str("device", session.DeviceID()).Int("bytes", len(data)).Int("total", session.AudioBuffer().Len()).Msg("audio frame received")
			// Feed decoded PCM to VAD pipeline (server-side speech end detection)
			if h.pipeline != nil && h.opusDecoder != nil {
				if err := h.feedVADPipeline(session, data); err != nil {
					h.log.Debug().Err(err).Str("device", session.DeviceID()).Msg("vad feed failed (frame dropped)")
				}
			}
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

func (h *Handler) handleHello(conn *websocket.Conn, session *ChatSession, raw []byte) {
	var msg protocol.HelloMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		h.log.Warn().Err(err).Msg("hello parse failed")
		return
	}
	resp := protocol.NewServerHello(session.ID(), protocol.AudioParams{
		Format: "opus", SampleRate: 24000, Channels: 1, FrameDuration: 60,
	})
	if err := conn.WriteJSON(resp); err != nil {
		h.log.Error().Err(err).Msg("hello response failed")
	}
	h.log.Info().Str("device", session.DeviceID()).Msg("hello exchanged")
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
		if h.interrupt != nil && h.interrupt.Triggered() {
			return
		}
		if h.interrupt != nil {
			h.interrupt.Trigger()
		}
		session.TransitionTo(StateListening)
	case protocol.ListenStateStop:
		audioBytes := session.AudioBuffer().Len()
		h.log.Info().Str("device", session.DeviceID()).Str("session", session.ID()).Int("audio_bytes", audioBytes).Msg("listen-stop received → triggering orchestrator")
		// 触发 orchestrator：drain 本轮 audio → STT → Chat → TTS → write back
		if err := h.invokeOrchestrator(conn, session); err != nil {
			h.log.Error().Err(err).Str("device", session.DeviceID()).Msg("orchestrator run failed")
		}
		session.TransitionTo(StateIdle)
	case protocol.ListenStateDetect:
		h.log.Debug().Str("device", session.DeviceID()).Msg("listen-detect (auto Wake Up)")
	}
}

// invokeOrchestrator 拉取本轮 audio bytes，调用 orchestrator.Run 走完整链路。
func (h *Handler) invokeOrchestrator(conn *websocket.Conn, session *ChatSession) error {
	if h.orchestrator == nil {
		return nil
	}
	device := session.Device()
	if device == nil {
		// 没 device 详情仍可跑：orchestrator 会用 fallback 模型。
		device = &aisaas.DeviceInfo{DeviceID: session.DeviceID()}
	}
	audioBytes := session.AudioBuffer().Drain()
	if len(audioBytes) == 0 {
		return nil
	}
	session.TransitionTo(StateThinking)
	defer func() { session.TransitionTo(StateIdle) }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return h.orchestrator.Run(ctx, session.ID(), conn, device, audioBytes)
}

func (h *Handler) handleAbort(session *ChatSession, raw []byte) {
	var msg protocol.AbortMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	data := session.AudioBuffer().Drain()
	if h.pipeline != nil && len(data) > 0 {
		h.pipeline.FeedBytes(session.ID(), data)
	}
	session.TransitionTo(StateIdle)
	h.log.Info().Str("reason", msg.Reason).Msg("abort received")
}

func (h *Handler) handleAck(session *ChatSession, raw []byte) {
	var msg protocol.AckMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	h.log.Debug().Str("msgType", msg.MsgType).Str("status", msg.Status).Msg("ack received")
}

// pcmAccumulator holds decoded PCM samples waiting to be fed to the VAD
// pipeline in 512-sample chunks. Each handler connection owns one.
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

// feedVADPipeline decodes one opus frame into PCM and pushes it to the
// per-session accumulator. While the accumulator holds ≥512 samples,
// they are pulled off in 32 ms chunks and handed to the VAD pipeline.
// The pipeline's internal state machine emits SpeechStart/SpeechEnd
// events which the handler consumes in pumpVADEvents.
func (h *Handler) feedVADPipeline(session *ChatSession, opusFrame []byte) error {
	pcm, err := h.opusDecoder.Decode(opusFrame)
	if err != nil {
		return err
	}
	if len(pcm) == 0 {
		return nil
	}
	v, _ := h.pcmAccum.LoadOrStore(session.ID(), &pcmAccumulator{})
	acc := v.(*pcmAccumulator)
	acc.push(pcm)
	for {
		chunk := acc.take(512)
		if chunk == nil {
			break
		}
		h.pipeline.Feed(session.ID(), chunk)
	}
	return nil
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
			bufBytes := session.AudioBuffer().Len()
			h.log.Info().Str("device", session.DeviceID()).Str("session", session.ID()).Int("audio_bytes", bufBytes).Msg("VAD speech end → triggering orchestrator")
			if err := h.invokeOrchestrator(conn, session); err != nil {
				h.log.Error().Err(err).Str("device", session.DeviceID()).Msg("vad-triggered orchestrator run failed")
			}
		}
	}
}
