package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/dialogue"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/event"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
)

type Handler struct {
	upgrader   websocket.Upgrader
	sm         *SessionManager
	aisaas     aisaas.Authenticator
	deviceInfo *aisaas.Client
	log        zerolog.Logger
	pipeline   *AudioPipeline
	interrupt  InterruptControllerRef
	eventBus   event.EventBusInterface
	orchestrator dialogue.OrchestratorRunner
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

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			h.log.Info().Err(err).Str("device", session.DeviceID()).Msg("read failed")
			return
		}
		session.Touch()

		if msgType == websocket.BinaryMessage {
			session.AudioBuffer().Write(data)
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
		return
	}
	switch msg.State {
	case protocol.ListenStateStart:
		if h.interrupt != nil && h.interrupt.Triggered() {
			return
		}
		if h.interrupt != nil {
			h.interrupt.Trigger()
		}
		session.TransitionTo(StateListening)
	case protocol.ListenStateStop:
		// 触发 orchestrator：drain 本轮 audio → STT → Chat → TTS → write back
		if err := h.invokeOrchestrator(conn, session); err != nil {
			h.log.Error().Err(err).Str("device", session.DeviceID()).Msg("orchestrator run failed")
		}
		session.TransitionTo(StateIdle)
	case protocol.ListenStateDetect:
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
