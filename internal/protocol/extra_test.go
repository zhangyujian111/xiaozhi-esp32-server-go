package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHelloMessage_Unmarshal_ServerToDevice(t *testing.T) {
	data := `{"type":"hello","transport":"websocket","session_id":"abc-123","audio_params":{"format":"opus","sample_rate":24000,"channels":1,"frame_duration":60}}`
	var msg HelloMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Hello, msg.Type)
	assert.Equal(t, "abc-123", msg.SessionID)
	assert.Equal(t, "websocket", msg.Transport)
	assert.Equal(t, 24000, msg.AudioParams.SampleRate)
	assert.Equal(t, 0, msg.Version)
	assert.Nil(t, msg.Features)
}

func TestNewServerHello(t *testing.T) {
	sessionID := "server-sess-xyz"
	params := AudioParams{Format: "opus", SampleRate: 24000, Channels: 1, FrameDuration: 60}
	msg := NewServerHello(sessionID, params)
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded HelloMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, Hello, msg.Type)
	assert.Equal(t, sessionID, msg.SessionID)
	assert.Equal(t, "websocket", msg.Transport)
	assert.Equal(t, 24000, msg.AudioParams.SampleRate)
	assert.Contains(t, string(data), `"session_id":"server-sess-xyz"`)
}

func TestListenMessage_RoundTrip(t *testing.T) {
	original := ListenMessage{Type: Listen, SessionID: "sess-listen", State: ListenStateStop, Mode: ListenModeRealtime, Text: ""}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded ListenMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, original.State, decoded.State)
	assert.Equal(t, original.Mode, decoded.Mode)
}

func TestBinaryV2Header_Offsets(t *testing.T) {
	header := make([]byte, 16)
	LittleEndian.PutUint16(header[0:2], 2)
	LittleEndian.PutUint16(header[2:4], 1)
	LittleEndian.PutUint32(header[4:8], 0x12345678)
	LittleEndian.PutUint32(header[8:12], 0xABCDEF00)
	LittleEndian.PutUint32(header[12:16], 100)
	h := BinaryV2Header{
		Version:     LittleEndian.Uint16(header[0:2]),
		Type:        LittleEndian.Uint16(header[2:4]),
		Reserved:    LittleEndian.Uint32(header[4:8]),
		Timestamp:   LittleEndian.Uint32(header[8:12]),
		PayloadSize: LittleEndian.Uint32(header[12:16]),
	}
	assert.Equal(t, uint16(2), h.Version)
	assert.Equal(t, uint16(1), h.Type)
	assert.Equal(t, uint32(0x12345678), h.Reserved)
	assert.Equal(t, uint32(0xABCDEF00), h.Timestamp)
	assert.Equal(t, uint32(100), h.PayloadSize)
}

func TestBinaryV3Header_Offsets(t *testing.T) {
	header := make([]byte, 4)
	header[0] = 3
	header[1] = 0xFF
	LittleEndian.PutUint16(header[2:4], 500)
	h := BinaryV3Header{
		Type:        header[0],
		Reserved:    header[1],
		PayloadSize: LittleEndian.Uint16(header[2:4]),
	}
	assert.Equal(t, uint8(3), h.Type)
	assert.Equal(t, uint8(0xFF), h.Reserved)
	assert.Equal(t, uint16(500), h.PayloadSize)
}
