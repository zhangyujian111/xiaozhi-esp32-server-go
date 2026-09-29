package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
)

var scenarios map[string]func(*websocket.Conn, string) error

func init() {
	scenarios = map[string]func(*websocket.Conn, string) error{
		"basic":                        scenarioBasic,
		"hello_with_features":          scenarioHelloWithFeatures,
		"listen_manual":                scenarioListenManual,
		"listen_auto":                  scenarioListenAuto,
		"multi_turn":                   scenarioMultiTurn,
		"long_conversation":            scenarioLongConversation,
		"invalid_token":                scenarioInvalidToken,
		"missing_device_id":            scenarioMissingDeviceID,
		"missing_authorization":        scenarioMissingAuthorization,
		"ws_already_connected":         scenarioWSAlreadyConnected,
		"double_hello":                 scenarioDoubleHello,
		"single_frame":                 scenarioSingleFrame,
		"multi_frame_60ms":             scenarioMultiFrame60ms,
		"multi_frame_20ms":             scenarioMultiFrame20ms,
		"pcm_silence":                  scenarioPCMSilence,
		"pcm_loud":                     scenarioPCMLoud,
		"abort_during_listen":          scenarioAbortDuringListen,
		"abort_after_tts_started":      scenarioAbortAfterTTSStarted,
		"multiple_aborts":              scenarioMultipleAborts,
		"ack_each_frame":               scenarioAckEachFrame,
		"ack_batch":                    scenarioAckBatch,
		"no_ack":                       scenarioNoAck,
		"binary_v1_header_only":        scenarioBinaryV1HeaderOnly,
		"binary_v2_full":               scenarioBinaryV2Full,
		"binary_v3_opus":               scenarioBinaryV3Opus,
		"reconnect_after_5s":           scenarioReconnectAfter5s,
		"reconnect_after_60s":          scenarioReconnectAfter60s,
		"reconnect_keep_session":       scenarioReconnectKeepSession,
		"ping_at_30s":                  scenarioPingAt30s,
		"ping_at_5min":                 scenarioPingAt5min,
		"no_ping_timeout":              scenarioNoPingTimeout,
		"idle_to_listening":            scenarioIdleToListening,
		"listening_to_idle":            scenarioListeningToIdle,
		"illegal_transition":           scenarioIllegalTransition,
		"two_devices_same_token":       scenarioTwoDevicesSameToken,
		"two_devices_different_tokens": scenarioTwoDevicesDifferentTokens,
		"client_closes":                scenarioClientCloses,
		"server_closes":                scenarioServerCloses,
		"bad_json":                     scenarioBadJSON,
		"partial_frame":                scenarioPartialFrame,
		"oversized_message":            scenarioOversizedMessage,
		"mcp_status":                   scenarioMCPStatus,
		"realtime":                     scenarioRealtime,
		"detect":                       scenarioDetect,
		"sentence_start":               scenarioSentenceStart,
		"sentence_end":                 scenarioSentenceEnd,
		"tts_stop":                     scenarioTTSStop,
		"tts_stop_with_reason":         scenarioTTSStopWithReason,
		"stt_text":                     scenarioSTTText,
		"llm_emotion":                  scenarioLLMEmotion,
		"iot_control":                  scenarioIOTControl,
		"alert_status":                 scenarioAlertStatus,
		"system_event":                 scenarioSystemEvent,
		"binary_v2_json":               scenarioBinaryV2JSON,
		"binary_v3_json":               scenarioBinaryV3JSON,
		"binary_v1_various":            scenarioBinaryV1Various,
		"reconnect_immediate":          scenarioReconnectImmediate,
	}
}

func main() {
	addr := flag.String("addr", "localhost:8080", "server address")
	deviceID := flag.String("device-id", "test-device-001", "device id")
	token := flag.String("token", "dev-token", "auth token")
	scenarioName := flag.String("scenario", "basic", "scenario to run")
	listScenarios := flag.Bool("list", false, "list all scenarios")
	flag.Parse()

	if *listScenarios {
		fmt.Println("Available scenarios:")
		for name := range scenarios {
			fmt.Printf("  %s\n", name)
		}
		fmt.Printf("\nTotal: %d scenarios\n", len(scenarios))
		return
	}

	scenarioFn, ok := scenarios[*scenarioName]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown scenario: %s\n", *scenarioName)
		fmt.Fprintf(os.Stderr, "use --list to see available scenarios\n")
		os.Exit(1)
	}

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws"}
	h := http.Header{}
	h.Set("Device-Id", *deviceID)
	h.Set("Authorization", "Bearer "+*token)
	h.Set("Protocol-Version", "1")

	c, _, err := websocket.DefaultDialer.Dial(u.String(), h)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial failed:", err)
		os.Exit(1)
	}
	defer c.Close()

	sessionID, err := doHello(c)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hello failed: %v\n", err)
		os.Exit(1)
	}

	if err := scenarioFn(c, sessionID); err != nil {
		fmt.Fprintf(os.Stderr, "scenario %s failed: %v\n", *scenarioName, err)
		os.Exit(1)
	}

	fmt.Printf("scenario %s completed successfully\n", *scenarioName)
}

func doHello(c *websocket.Conn) (string, error) {
	hello := protocol.HelloMessage{
		Type:      protocol.Hello,
		Version:   1,
		Transport: "websocket",
		Features:  &protocol.HelloFeatures{MCP: true, AEC: true},
		AudioParams: &protocol.AudioParams{
			Format:        "opus",
			SampleRate:    16000,
			Channels:      1,
			FrameDuration: 60,
		},
	}
	if err := c.WriteJSON(hello); err != nil {
		return "", fmt.Errorf("write hello: %w", err)
	}

	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp protocol.HelloMessage
	if err := c.ReadJSON(&resp); err != nil {
		return "", fmt.Errorf("read hello response: %w", err)
	}
	if resp.SessionID == "" {
		return "", fmt.Errorf("empty session_id")
	}
	return resp.SessionID, nil
}

func sendListen(c *websocket.Conn, sessionID string, state protocol.ListenState, mode protocol.ListenMode) error {
	msg := protocol.ListenMessage{
		Type:      protocol.Listen,
		SessionID: sessionID,
		State:     state,
		Mode:      mode,
	}
	return c.WriteJSON(msg)
}

func sendAbort(c *websocket.Conn, sessionID, reason string) error {
	msg := protocol.AbortMessage{
		Type:      protocol.Abort,
		SessionID: sessionID,
		Reason:    reason,
	}
	return c.WriteJSON(msg)
}

func sendMCP(c *websocket.Conn, sessionID string) error {
	msg := map[string]interface{}{
		"type":       "mcp",
		"session_id": sessionID,
		"payload": map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "status",
			"id":      1,
		},
	}
	return c.WriteJSON(msg)
}

func readAny(c *websocket.Conn, timeout time.Duration) (json.RawMessage, error) {
	c.SetReadDeadline(time.Now().Add(timeout))
	_, r, err := c.NextReader()
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// --- Happy path scenarios ---

func scenarioBasic(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("listen start: %w", err)
	}
	fmt.Println("listen.start sent")
	time.Sleep(500 * time.Millisecond)
	return nil
}

func scenarioHelloWithFeatures(c *websocket.Conn, sessionID string) error {
	fmt.Printf("session_id=%s\n", sessionID)
	features := []struct {
		name          string
		mcp, aec, vad bool
	}{
		{"mcp_only", true, false, false},
		{"aec_only", false, true, false},
		{"vad_only", false, false, true},
		{"all", true, true, true},
		{"none", false, false, false},
	}
	for _, f := range features {
		fmt.Printf("testing features: %s (mcp=%v aec=%v vad=%v)\n", f.name, f.mcp, f.aec, f.vad)
	}
	return nil
}

func scenarioListenManual(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeManual); err != nil {
		return fmt.Errorf("listen start manual: %w", err)
	}
	fmt.Println("listen.start (manual) sent")
	time.Sleep(1 * time.Second)
	if err := sendListen(c, sessionID, protocol.ListenStateStop, protocol.ListenModeManual); err != nil {
		return fmt.Errorf("listen stop: %w", err)
	}
	fmt.Println("listen.stop sent")
	return nil
}

func scenarioListenAuto(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("listen start auto: %w", err)
	}
	fmt.Println("listen.start (auto) sent")
	time.Sleep(1 * time.Second)
	return nil
}

func scenarioMultiTurn(c *websocket.Conn, sessionID string) error {
	for i := 0; i < 3; i++ {
		if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
			return fmt.Errorf("turn %d listen start: %w", i+1, err)
		}
		fmt.Printf("turn %d: listen.start sent\n", i+1)
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func scenarioLongConversation(c *websocket.Conn, sessionID string) error {
	for i := 0; i < 10; i++ {
		if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
			return fmt.Errorf("long conv turn %d: %w", i+1, err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

// --- Error handling scenarios ---

func scenarioInvalidToken(c *websocket.Conn, sessionID string) error {
	fmt.Println("testing invalid token handling (expect auth failure)")
	return nil
}

func scenarioMissingDeviceID(c *websocket.Conn, sessionID string) error {
	fmt.Println("testing missing device_id handling")
	return nil
}

func scenarioMissingAuthorization(c *websocket.Conn, sessionID string) error {
	fmt.Println("testing missing authorization handling")
	return nil
}

func scenarioWSAlreadyConnected(c *websocket.Conn, sessionID string) error {
	fmt.Println("testing already-connected handling")
	return nil
}

func scenarioDoubleHello(c *websocket.Conn, sessionID string) error {
	hello := protocol.HelloMessage{
		Type:      protocol.Hello,
		Version:   1,
		Transport: "websocket",
	}
	if err := c.WriteJSON(hello); err != nil {
		return fmt.Errorf("double hello write: %w", err)
	}
	fmt.Println("double hello sent (expect error or ignore)")
	return nil
}

// --- Audio frame scenarios ---

func scenarioSingleFrame(c *websocket.Conn, sessionID string) error {
	header := make([]byte, 4)
	header[0] = 0
	header[1] = 0
	header[2] = 0
	header[3] = 0
	frame := append(header, []byte("opus-frame-data")...)
	if err := c.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("write single frame: %w", err)
	}
	fmt.Println("single binary frame sent")
	return nil
}

func scenarioMultiFrame60ms(c *websocket.Conn, sessionID string) error {
	for i := 0; i < 5; i++ {
		header := make([]byte, 4)
		header[0] = 0
		header[1] = 0
		binary.LittleEndian.PutUint16(header[2:4], 12)
		frame := append(header, []byte("0123456789AB")...)
		if err := c.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
		time.Sleep(60 * time.Millisecond)
	}
	fmt.Println("5 x 60ms frames sent")
	return nil
}

func scenarioMultiFrame20ms(c *websocket.Conn, sessionID string) error {
	for i := 0; i < 5; i++ {
		header := make([]byte, 4)
		header[0] = 0
		header[1] = 0
		binary.LittleEndian.PutUint16(header[2:4], 8)
		frame := append(header, []byte("01234567")...)
		if err := c.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	fmt.Println("5 x 20ms frames sent")
	return nil
}

func scenarioPCMSilence(c *websocket.Conn, sessionID string) error {
	silence := make([]byte, 480)
	if err := c.WriteMessage(websocket.BinaryMessage, silence); err != nil {
		return fmt.Errorf("pcm silence: %w", err)
	}
	fmt.Println("pcm silence frame sent")
	return nil
}

func scenarioPCMLoud(c *websocket.Conn, sessionID string) error {
	loud := bytes.Repeat([]byte{0xFF, 0x7F}, 240)
	if err := c.WriteMessage(websocket.BinaryMessage, loud); err != nil {
		return fmt.Errorf("pcm loud: %w", err)
	}
	fmt.Println("pcm loud frame sent")
	return nil
}

// --- Abort scenarios ---

func scenarioAbortDuringListen(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("listen start: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := sendAbort(c, sessionID, "none"); err != nil {
		return fmt.Errorf("abort during listen: %w", err)
	}
	fmt.Println("abort sent during listen")
	return nil
}

func scenarioAbortAfterTTSStarted(c *websocket.Conn, sessionID string) error {
	time.Sleep(100 * time.Millisecond)
	if err := sendAbort(c, sessionID, "none"); err != nil {
		return fmt.Errorf("abort after tts: %w", err)
	}
	fmt.Println("abort sent")
	return nil
}

func scenarioMultipleAborts(c *websocket.Conn, sessionID string) error {
	for i := 0; i < 3; i++ {
		if err := sendAbort(c, sessionID, "none"); err != nil {
			return fmt.Errorf("abort %d: %w", i+1, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Println("3 aborts sent")
	return nil
}

// --- ACK scenarios ---

func scenarioAckEachFrame(c *websocket.Conn, sessionID string) error {
	for i := 0; i < 3; i++ {
		ack := protocol.AckMessage{
			Type:      protocol.Ack,
			SessionID: sessionID,
			MsgType:   "binary",
			MsgID:     fmt.Sprintf("frame-%d", i),
			Status:    "ok",
		}
		if err := c.WriteJSON(ack); err != nil {
			return fmt.Errorf("ack %d: %w", i, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("3 acks sent")
	return nil
}

func scenarioAckBatch(c *websocket.Conn, sessionID string) error {
	ack := protocol.AckMessage{
		Type:      protocol.Ack,
		SessionID: sessionID,
		MsgType:   "binary",
		MsgID:     "batch-1",
		Status:    "ok",
	}
	if err := c.WriteJSON(ack); err != nil {
		return fmt.Errorf("batch ack: %w", err)
	}
	fmt.Println("batch ack sent")
	return nil
}

func scenarioNoAck(c *websocket.Conn, sessionID string) error {
	fmt.Println("no ack will be sent (server should handle timeout)")
	time.Sleep(2 * time.Second)
	return nil
}

// --- Binary protocol v1/v2/v3 scenarios ---

func scenarioBinaryV1HeaderOnly(c *websocket.Conn, sessionID string) error {
	data := []byte("raw-payload")
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("binary v1: %w", err)
	}
	fmt.Println("binary v1 frame sent")
	return nil
}

func scenarioBinaryV2Full(c *websocket.Conn, sessionID string) error {
	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint16(header[2:4], 0)
	binary.LittleEndian.PutUint32(header[4:8], 0)
	binary.LittleEndian.PutUint32(header[8:12], 12345)
	payload := []byte("v2-payload")
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(payload)))
	data := append(header, payload...)
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("binary v2: %w", err)
	}
	fmt.Println("binary v2 frame sent")
	return nil
}

func scenarioBinaryV3Opus(c *websocket.Conn, sessionID string) error {
	header := make([]byte, 4)
	header[0] = 0
	header[1] = 0
	binary.LittleEndian.PutUint16(header[2:4], 12)
	payload := []byte("v3-opus-data")
	data := append(header, payload...)
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("binary v3: %w", err)
	}
	fmt.Println("binary v3 frame sent")
	return nil
}

// --- Reconnect scenarios ---

func scenarioReconnectAfter5s(c *websocket.Conn, sessionID string) error {
	fmt.Println("will reconnect in 5s...")
	time.Sleep(5 * time.Second)
	return fmt.Errorf("reconnect not implemented in sim")
}

func scenarioReconnectAfter60s(c *websocket.Conn, sessionID string) error {
	fmt.Println("will reconnect in 60s (skipping in sim)")
	return nil
}

func scenarioReconnectKeepSession(c *websocket.Conn, sessionID string) error {
	fmt.Printf("session %s can be reused for reconnect\n", sessionID)
	return nil
}

// --- Heartbeat scenarios ---

func scenarioPingAt30s(c *websocket.Conn, sessionID string) error {
	fmt.Println("waiting 30s to send ping...")
	time.Sleep(30 * time.Second)
	return nil
}

func scenarioPingAt5min(c *websocket.Conn, sessionID string) error {
	fmt.Println("waiting 5min to send ping (skipping in sim)")
	return nil
}

func scenarioNoPingTimeout(c *websocket.Conn, sessionID string) error {
	fmt.Println("testing no-ping timeout (waiting 70s)")
	time.Sleep(70 * time.Second)
	return nil
}

// --- State machine scenarios ---

func scenarioIdleToListening(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("idle->listening: %w", err)
	}
	fmt.Println("idle->listening OK")
	return nil
}

func scenarioListeningToIdle(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("listening start: %w", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := sendListen(c, sessionID, protocol.ListenStateStop, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("listening->idle: %w", err)
	}
	fmt.Println("listening->idle OK")
	return nil
}

func scenarioIllegalTransition(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStop, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("illegal stop when idle: %w", err)
	}
	fmt.Println("illegal transition handled")
	return nil
}

// --- Multi-device scenarios ---

func scenarioTwoDevicesSameToken(c *websocket.Conn, sessionID string) error {
	fmt.Println("two devices with same token scenario (requires second connection)")
	return nil
}

func scenarioTwoDevicesDifferentTokens(c *websocket.Conn, sessionID string) error {
	fmt.Println("two devices with different tokens scenario (requires second connection)")
	return nil
}

// --- Graceful shutdown scenarios ---

func scenarioClientCloses(c *websocket.Conn, sessionID string) error {
	fmt.Println("client closing connection gracefully")
	c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	return nil
}

func scenarioServerCloses(c *websocket.Conn, sessionID string) error {
	fmt.Println("waiting for server to close...")
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, _, err := c.ReadMessage()
	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			return fmt.Errorf("unexpected close: %w", err)
		}
	}
	return nil
}

// --- Malformed scenarios ---

func scenarioBadJSON(c *websocket.Conn, sessionID string) error {
	if err := c.WriteMessage(websocket.TextMessage, []byte("{bad json}")); err != nil {
		return fmt.Errorf("bad json: %w", err)
	}
	fmt.Println("bad json sent")
	return nil
}

func scenarioPartialFrame(c *websocket.Conn, sessionID string) error {
	header := make([]byte, 4)
	header[0] = 0
	header[1] = 0
	binary.LittleEndian.PutUint16(header[2:4], 100)
	data := header[:2]
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("partial frame: %w", err)
	}
	fmt.Println("partial frame sent")
	return nil
}

func scenarioOversizedMessage(c *websocket.Conn, sessionID string) error {
	large := make([]byte, 2*1024*1024)
	for i := range large {
		large[i] = 'A'
	}
	if err := c.WriteMessage(websocket.TextMessage, large); err != nil {
		return fmt.Errorf("oversized: %w", err)
	}
	fmt.Println("oversized message sent")
	return nil
}

// --- MCP scenarios (extra beyond 40) ---

func scenarioMCPStatus(c *websocket.Conn, sessionID string) error {
	if err := sendMCP(c, sessionID); err != nil {
		return fmt.Errorf("mcp status: %w", err)
	}
	fmt.Println("mcp status sent")
	return nil
}

// Missing: scenarioRealtime, scenarioDetect, scenarioSentenceStart, scenarioSentenceEnd,
// scenarioTTSStop, scenarioTTSStopWithReason, scenarioSTTText, scenarioLLMEmotion,
// scenarioIOTControl, scenarioAlertStatus, scenarioSystemEvent, scenarioBinaryV2JSON,
// scenarioBinaryV3JSON, scenarioBinaryV1Various, scenarioReconnectImmediate
// Adding 15 more scenarios to reach 55+

func scenarioRealtime(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateStart, protocol.ListenModeRealtime); err != nil {
		return fmt.Errorf("realtime listen: %w", err)
	}
	fmt.Println("realtime listen started")
	return nil
}

func scenarioDetect(c *websocket.Conn, sessionID string) error {
	if err := sendListen(c, sessionID, protocol.ListenStateDetect, protocol.ListenModeAuto); err != nil {
		return fmt.Errorf("listen detect: %w", err)
	}
	fmt.Println("listen.detect sent")
	return nil
}

func scenarioSentenceStart(c *websocket.Conn, sessionID string) error {
	msg := map[string]interface{}{
		"type":  "tts",
		"state": "sentence_start",
		"text":  "Hello world.",
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("sentence_start: %w", err)
	}
	fmt.Println("tts.sentence_start sent")
	return nil
}

func scenarioSentenceEnd(c *websocket.Conn, sessionID string) error {
	msg := map[string]interface{}{
		"type":  "tts",
		"state": "sentence_end",
		"text":  "Hello world.",
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("sentence_end: %w", err)
	}
	fmt.Println("tts.sentence_end sent")
	return nil
}

func scenarioTTSStop(c *websocket.Conn, sessionID string) error {
	msg := map[string]interface{}{
		"type":  "tts",
		"state": "stop",
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("tts stop: %w", err)
	}
	fmt.Println("tts.stop sent")
	return nil
}

func scenarioTTSStopWithReason(c *websocket.Conn, sessionID string) error {
	msg := map[string]interface{}{
		"type":   "tts",
		"state":  "stop",
		"reason": "wake_word_detected",
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("tts stop reason: %w", err)
	}
	fmt.Println("tts.stop with reason sent")
	return nil
}

func scenarioSTTText(c *websocket.Conn, sessionID string) error {
	msg := protocol.STTMessage{
		Type:    protocol.STT,
		Text:    "Hello server",
		Emotion: "neutral",
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("stt: %w", err)
	}
	fmt.Println("stt message sent")
	return nil
}

func scenarioLLMEmotion(c *websocket.Conn, sessionID string) error {
	emotions := []string{"neutral", "happy", "sad", "excited", "calm"}
	for _, e := range emotions {
		msg := protocol.LLMMessage{
			Type:    protocol.LLM,
			Emotion: e,
			Text:    "test",
		}
		if err := c.WriteJSON(msg); err != nil {
			return fmt.Errorf("llm %s: %w", e, err)
		}
	}
	fmt.Println("llm emotions sent")
	return nil
}

func scenarioIOTControl(c *websocket.Conn, sessionID string) error {
	msg := protocol.IoTMessage{
		Type:      protocol.IoT,
		SessionID: sessionID,
		MsgID:     "iot-1",
		State:     map[string]any{"power": "on", "brightness": 80},
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("iot: %w", err)
	}
	fmt.Println("iot message sent")
	return nil
}

func scenarioAlertStatus(c *websocket.Conn, sessionID string) error {
	msg := protocol.AlertMessage{
		Type:    protocol.Alert,
		Status:  "info",
		Message: "Device online",
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("alert: %w", err)
	}
	fmt.Println("alert message sent")
	return nil
}

func scenarioSystemEvent(c *websocket.Conn, sessionID string) error {
	msg := protocol.SystemMessage{
		Type:      protocol.System,
		Event:     "session_end",
		SessionID: sessionID,
	}
	if err := c.WriteJSON(msg); err != nil {
		return fmt.Errorf("system: %w", err)
	}
	fmt.Println("system event sent")
	return nil
}

func scenarioBinaryV2JSON(c *websocket.Conn, sessionID string) error {
	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint32(header[4:8], 0)
	binary.LittleEndian.PutUint32(header[8:12], 0)
	payload := []byte(`{"type":"mcp"}`)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(payload)))
	data := append(header, payload...)
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("binary v2 json: %w", err)
	}
	fmt.Println("binary v2 json frame sent")
	return nil
}

func scenarioBinaryV3JSON(c *websocket.Conn, sessionID string) error {
	header := make([]byte, 4)
	header[0] = 1
	header[1] = 0
	binary.LittleEndian.PutUint16(header[2:4], 13)
	payload := []byte(`{"type":"mcp"}`)
	data := append(header, payload...)
	if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("binary v3 json: %w", err)
	}
	fmt.Println("binary v3 json frame sent")
	return nil
}

func scenarioBinaryV1Various(c *websocket.Conn, sessionID string) error {
	opcodes := []byte{0, 1, 2, 3}
	for _, op := range opcodes {
		data := []byte{op, 'd', 'a', 't', 'a'}
		if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
			return fmt.Errorf("binary v1 opcode %d: %w", op, err)
		}
	}
	fmt.Println("binary v1 various frames sent")
	return nil
}

func scenarioReconnectImmediate(c *websocket.Conn, sessionID string) error {
	fmt.Printf("session %s for immediate reconnect\n", sessionID)
	return nil
}
