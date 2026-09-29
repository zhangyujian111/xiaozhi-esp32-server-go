package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHelloMessage_Unmarshal_DeviceToServer(t *testing.T) {
	data := `{"type":"hello","version":1,"features":{"mcp":true,"aec":true},"transport":"websocket","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60}}`
	var msg HelloMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Hello, msg.Type)
	assert.Equal(t, 1, msg.Version)
	assert.NotNil(t, msg.Features)
	assert.True(t, msg.Features.MCP)
	assert.True(t, msg.Features.AEC)
	assert.Equal(t, "websocket", msg.Transport)
	assert.NotNil(t, msg.AudioParams)
	assert.Equal(t, "opus", msg.AudioParams.Format)
	assert.Equal(t, 16000, msg.AudioParams.SampleRate)
	assert.Equal(t, 1, msg.AudioParams.Channels)
	assert.Equal(t, 60, msg.AudioParams.FrameDuration)
	assert.Empty(t, msg.SessionID)
}

func TestHelloMessage_Unmarshal_DeviceToServer_MissingSessionID(t *testing.T) {
	data := `{"type":"hello","version":1,"features":{"mcp":false},"transport":"websocket","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60}}`
	var msg HelloMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Hello, msg.Type)
	assert.Equal(t, 1, msg.Version)
	assert.NotNil(t, msg.Features)
	assert.False(t, msg.Features.MCP)
	assert.False(t, msg.Features.AEC)
	assert.Empty(t, msg.SessionID)
}

func TestHelloMessage_Marshal_ServerToDevice(t *testing.T) {
	sessionID := "test-session-123"
	audioParams := AudioParams{
		Format:        "opus",
		SampleRate:    24000,
		Channels:      1,
		FrameDuration: 60,
	}
	msg := NewServerHello(sessionID, audioParams)
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded HelloMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip unmarshal failed: %v", err)
	}
	assert.Equal(t, Hello, decoded.Type)
	assert.Equal(t, sessionID, decoded.SessionID)
	assert.Equal(t, "websocket", decoded.Transport)
	assert.Equal(t, audioParams.Format, decoded.AudioParams.Format)
	assert.Equal(t, audioParams.SampleRate, decoded.AudioParams.SampleRate)
	assert.Equal(t, audioParams.Channels, decoded.AudioParams.Channels)
	assert.Equal(t, audioParams.FrameDuration, decoded.AudioParams.FrameDuration)
	assert.Equal(t, 0, decoded.Version)
	assert.Nil(t, decoded.Features)
}

func TestHelloMessage_RoundTrip_JSON(t *testing.T) {
	original := `{"type":"hello","version":1,"features":{"mcp":true,"aec":false,"vad":true},"transport":"websocket","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60}}`
	var msg HelloMessage
	err := json.Unmarshal([]byte(original), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded HelloMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip unmarshal failed: %v", err)
	}
	assert.Equal(t, Hello, decoded.Type)
	assert.Equal(t, 1, decoded.Version)
	assert.True(t, decoded.Features.MCP)
	assert.False(t, decoded.Features.AEC)
	assert.True(t, decoded.Features.VAD)
	assert.Equal(t, "websocket", decoded.Transport)
	assert.Equal(t, "opus", decoded.AudioParams.Format)
	assert.Equal(t, 16000, decoded.AudioParams.SampleRate)
}
