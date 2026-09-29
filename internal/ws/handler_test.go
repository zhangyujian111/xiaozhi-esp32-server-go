package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
)

type mockAuth struct {
	err error
}

func (m *mockAuth) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
	return m.err
}

func TestWebSocketUpgrade_MissingDeviceID(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()
	h.HandleUpgrade(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWebSocketUpgrade_MissingAuthorization(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Device-Id", "test-device")
	rec := httptest.NewRecorder()
	h.HandleUpgrade(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWebSocketUpgrade_InvalidToken(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: aisaas.ErrNotImplemented}, logger)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Device-Id", "test-device")
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	h.HandleUpgrade(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWebSocketUpgrade_Valid(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))

	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	hello := &protocol.HelloMessage{
		Type:      protocol.Hello,
		Version:   1,
		Transport: "websocket",
		AudioParams: &protocol.AudioParams{
			Format:        "opus",
			SampleRate:    16000,
			Channels:      1,
			FrameDuration: 60,
		},
	}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}

	select {
	case resp := <-msgCh:
		if resp.SessionID == "" {
			t.Error("expected non-empty session_id in hello response")
		}
		if resp.AudioParams == nil {
			t.Error("expected audio_params in hello response")
		} else if resp.AudioParams.SampleRate != 24000 {
			t.Errorf("expected sample_rate=24000, got %d", resp.AudioParams.SampleRate)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for hello response")
	}

	if count := sm.Count(); count != 1 {
		t.Fatalf("expected 1 session in manager, got %d", count)
	}
}

func TestHandleHello_RoundTrip(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	hello := &protocol.HelloMessage{
		Type:      protocol.Hello,
		Version:   1,
		Transport: "websocket",
		AudioParams: &protocol.AudioParams{
			Format:        "opus",
			SampleRate:    16000,
			Channels:      1,
			FrameDuration: 60,
		},
	}
	if err := conn.WriteJSON(hello); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}

	select {
	case resp := <-msgCh:
		if resp.AudioParams == nil {
			t.Fatal("expected audio_params in response")
		}
		if resp.AudioParams.SampleRate != 24000 {
			t.Errorf("expected sample_rate=24000, got %d", resp.AudioParams.SampleRate)
		}
		if resp.AudioParams.Format != "opus" {
			t.Errorf("expected format=opus, got %s", resp.AudioParams.Format)
		}
		if resp.AudioParams.Channels != 1 {
			t.Errorf("expected channels=1, got %d", resp.AudioParams.Channels)
		}
		if resp.AudioParams.FrameDuration != 60 {
			t.Errorf("expected frame_duration=60, got %d", resp.AudioParams.FrameDuration)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for hello response")
	}
}

func TestHandleListen_TransitionsState(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	conn.WriteJSON(&protocol.HelloMessage{Type: protocol.Hello, Version: 1, Transport: "websocket"})
	<-msgCh

	session, _ := sm.GetByDevice("test-device")
	sessionID := session.ID()

	listenStart := &protocol.ListenMessage{Type: protocol.Listen, SessionID: sessionID, State: protocol.ListenStateStart, Mode: protocol.ListenModeAuto}
	if err := conn.WriteJSON(listenStart); err != nil {
		t.Fatalf("write listen.start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	s, _ := sm.Get(sessionID)
	if s == nil {
		t.Fatal("session not found")
	}
	if s.State() != StateListening {
		t.Errorf("expected state=listening after listen.start, got %s", s.State())
	}

	listenStop := &protocol.ListenMessage{Type: protocol.Listen, SessionID: sessionID, State: protocol.ListenStateStop}
	if err := conn.WriteJSON(listenStop); err != nil {
		t.Fatalf("write listen.stop failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	s, _ = sm.Get(sessionID)
	if s == nil {
		t.Fatal("session not found")
	}
	if s.State() != StateIdle {
		t.Errorf("expected state=idle after listen.stop, got %s", s.State())
	}
}

func TestHandleAbort_ResetsToIdle(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	conn.WriteJSON(&protocol.HelloMessage{Type: protocol.Hello, Version: 1, Transport: "websocket"})
	<-msgCh

	session, _ := sm.GetByDevice("test-device")
	sessionID := session.ID()

	s, _ := sm.Get(sessionID)
	s.TransitionTo(StateThinking)
	s.AudioBuffer().Write([]byte{1, 2, 3})

	abortMsg := &protocol.AbortMessage{Type: protocol.Abort, SessionID: sessionID, Reason: "user cancel"}
	if err := conn.WriteJSON(abortMsg); err != nil {
		t.Fatalf("write abort failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	s, _ = sm.Get(sessionID)
	if s == nil {
		t.Fatal("session not found")
	}
	if s.State() != StateIdle {
		t.Errorf("expected state=idle after abort, got %s", s.State())
	}
	if s.AudioBuffer().Len() != 0 {
		t.Errorf("expected audio buffer empty after abort, got len=%d", s.AudioBuffer().Len())
	}
}

func TestHandleAck_LogsMsgType(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	conn.WriteJSON(&protocol.HelloMessage{Type: protocol.Hello, Version: 1, Transport: "websocket"})
	<-msgCh

	session, _ := sm.GetByDevice("test-device")
	sessionID := session.ID()

	ackMsg := &protocol.AckMessage{Type: protocol.Ack, SessionID: sessionID, MsgType: "tts", MsgID: "msg-123", Status: "complete"}
	if err := conn.WriteJSON(ackMsg); err != nil {
		t.Fatalf("write ack failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
}

func TestHandleMCP_LogsAckOnly(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	conn.WriteJSON(&protocol.HelloMessage{Type: protocol.Hello, Version: 1, Transport: "websocket"})
	<-msgCh

	mcpMsg := map[string]interface{}{"type": "mcp", "session_id": "s1"}
	if err := conn.WriteJSON(mcpMsg); err != nil {
		t.Fatalf("write mcp failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
}

func TestServeConn_BinaryMessageWrittenToBuffer(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	conn.WriteJSON(&protocol.HelloMessage{Type: protocol.Hello, Version: 1, Transport: "websocket"})
	<-msgCh

	session, _ := sm.GetByDevice("test-device")
	sessionID := session.ID()

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3, 4, 5}); err != nil {
		t.Fatalf("write binary failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	s, _ := sm.Get(sessionID)
	if s == nil {
		t.Fatal("session not found")
	}
	if s.AudioBuffer().Len() != 5 {
		t.Errorf("expected audio buffer len=5 after binary write, got %d", s.AudioBuffer().Len())
	}
}

func TestServeConn_UnknownMessageType(t *testing.T) {
	sm := NewSessionManager()
	logger := zerolog.Nop()
	h := NewHandler(sm, &mockAuth{err: nil}, logger)

	server := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer server.Close()

	wsURL := "ws://" + server.URL[7:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Device-Id":     {"test-device"},
		"Authorization": {"Bearer valid-token"},
	})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	msgCh := make(chan *protocol.HelloMessage, 1)
	go func() {
		var resp protocol.HelloMessage
		if err := conn.ReadJSON(&resp); err == nil {
			msgCh <- &resp
		}
	}()

	conn.WriteJSON(&protocol.HelloMessage{Type: protocol.Hello, Version: 1, Transport: "websocket"})
	<-msgCh

	unknownMsg := map[string]interface{}{"type": "unknown_type_xyz"}
	if err := conn.WriteJSON(unknownMsg); err != nil {
		t.Fatalf("write unknown msg failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
}
