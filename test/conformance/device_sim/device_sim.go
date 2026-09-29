package main

import (
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

func main() {
	addr := flag.String("addr", "localhost:8080", "server address")
	deviceID := flag.String("device-id", "test-device-001", "device id")
	token := flag.String("token", "dev-token", "auth token")
	flag.Parse()

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
		fmt.Fprintln(os.Stderr, "hello failed:", err)
		os.Exit(1)
	}
	fmt.Println("hello sent, waiting for response...")

	c.SetReadDeadline(time.Now().Add(5 * time.Second))

	var resp protocol.HelloMessage
	if err := c.ReadJSON(&resp); err != nil {
		fmt.Fprintln(os.Stderr, "read failed:", err)
		os.Exit(1)
	}
	fmt.Printf("hello response: session_id=%s sample_rate=%d\n", resp.SessionID, resp.AudioParams.SampleRate)

	if resp.SessionID == "" {
		fmt.Fprintln(os.Stderr, "error: empty session_id in response")
		os.Exit(1)
	}

	listen := protocol.ListenMessage{
		Type:      protocol.Listen,
		SessionID: resp.SessionID,
		State:     protocol.ListenStateStart,
		Mode:      protocol.ListenModeAuto,
	}
	if err := c.WriteJSON(listen); err != nil {
		fmt.Fprintln(os.Stderr, "listen.start failed:", err)
		os.Exit(1)
	}
	fmt.Println("listen.start sent")

	time.Sleep(2 * time.Second)
	fmt.Println("done")
}

type envelope struct {
	Type string `json:"type"`
}

func sendMCP(c *websocket.Conn, sessionID string) error {
	mcp := map[string]interface{}{
		"type":       "mcp",
		"session_id": sessionID,
		"action":     "status",
	}
	return c.WriteJSON(mcp)
}

func sendIoT(c *websocket.Conn, sessionID string) error {
	iot := map[string]interface{}{
		"type":       "iot",
		"session_id": sessionID,
		"device":     "light-1",
		"action":     "on",
	}
	return c.WriteJSON(iot)
}

func handleUnknown(c *websocket.Conn, raw []byte) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		fmt.Fprintf(os.Stderr, "malformed JSON: %v\n", err)
		return
	}
	fmt.Printf("unknown message type: %s\n", env.Type)
}
