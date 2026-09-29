package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListenMessage_Unmarshal_DeviceToServer(t *testing.T) {
	data := `{"type":"listen","session_id":"sess-123","state":"start","mode":"auto","text":""}`
	var msg ListenMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Listen, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.Equal(t, ListenStateStart, msg.State)
	assert.Equal(t, ListenModeAuto, msg.Mode)
	assert.Equal(t, "", msg.Text)
}

func TestListenMessage_Marshal_ServerToDevice(t *testing.T) {
	msg := ListenMessage{
		Type:      Listen,
		SessionID: "sess-456",
		State:     ListenStateStart,
		Mode:      ListenModeManual,
		Text:      "",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded ListenMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, Listen, decoded.Type)
	assert.Equal(t, "sess-456", decoded.SessionID)
	assert.Equal(t, ListenStateStart, decoded.State)
	assert.Equal(t, ListenModeManual, decoded.Mode)
}

func TestListenMessage_Unmarshal_StateDetect(t *testing.T) {
	data := `{"type":"listen","session_id":"sess-789","state":"detect","text":"小智同学"}`
	var msg ListenMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, ListenStateDetect, msg.State)
	assert.Equal(t, "小智同学", msg.Text)
}

func TestAbortMessage_Unmarshal_DeviceToServer(t *testing.T) {
	data := `{"type":"abort","session_id":"sess-123","reason":"wake_word_detected"}`
	var msg AbortMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Abort, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.Equal(t, "wake_word_detected", msg.Reason)
}

func TestAbortMessage_Unmarshal_NoReason(t *testing.T) {
	data := `{"type":"abort","session_id":"sess-123"}`
	var msg AbortMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Abort, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.Empty(t, msg.Reason)
}

func TestAckMessage_Unmarshal_DeviceToServer(t *testing.T) {
	data := `{"type":"ack","sessionId":"sess-123","msgType":"tts","msgId":"msg-456","status":"ok","error":""}`
	var msg AckMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Ack, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.Equal(t, "tts", msg.MsgType)
	assert.Equal(t, "msg-456", msg.MsgID)
	assert.Equal(t, "ok", msg.Status)
	assert.Empty(t, msg.Error)
}

func TestAckMessage_Unmarshal_WithError(t *testing.T) {
	data := `{"type":"ack","sessionId":"sess-123","msgType":"llm","msgId":"msg-789","status":"error","error":"model unavailable"}`
	var msg AckMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, "error", msg.Status)
	assert.Equal(t, "model unavailable", msg.Error)
}

func TestAckMessage_Marshal_ServerToDevice(t *testing.T) {
	msg := AckMessage{
		Type:      Ack,
		SessionID: "sess-abc",
		MsgType:   "stt",
		MsgID:     "msg-def",
		Status:    "ok",
		Error:     "",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded AckMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, Ack, decoded.Type)
	assert.Equal(t, "sess-abc", decoded.SessionID)
	assert.Equal(t, "stt", decoded.MsgType)
	assert.Equal(t, "msg-def", decoded.MsgID)
	assert.Equal(t, "ok", decoded.Status)
}
