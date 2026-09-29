package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPMessage_Unmarshal(t *testing.T) {
	data := `{"type":"mcp","session_id":"sess-123","payload":{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}}`
	var msg MCPMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, MCP, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.NotNil(t, msg.Payload)
	assert.Equal(t, "2.0", msg.Payload.JSONRPC)
	assert.Equal(t, float64(1), msg.Payload.ID)
	assert.Equal(t, "tools/list", msg.Payload.Method)
}

func TestMCPMessage_RoundTrip(t *testing.T) {
	payload := MCPJSONRPC{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test"}`),
	}
	original := MCPMessage{
		Type:      MCP,
		SessionID: "sess-abc",
		Payload:   &payload,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded MCPMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, MCP, decoded.Type)
	assert.Equal(t, "sess-abc", decoded.SessionID)
	assert.NotNil(t, decoded.Payload)
	assert.Equal(t, "2.0", decoded.Payload.JSONRPC)
	assert.Equal(t, float64(42), decoded.Payload.ID)
	assert.Equal(t, "tools/call", decoded.Payload.Method)
}

func TestMCPMessage_Response(t *testing.T) {
	data := `{"type":"mcp","session_id":"sess-123","payload":{"jsonrpc":"2.0","id":5,"result":{"tools":[{"name":"weather"}]}}}`
	var msg MCPMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.NotNil(t, msg.Payload)
	assert.NotNil(t, msg.Payload.Result)
	assert.Nil(t, msg.Payload.Error)
}

func TestMCPMessage_ErrorResponse(t *testing.T) {
	data := `{"type":"mcp","session_id":"sess-123","payload":{"jsonrpc":"2.0","id":5,"error":{"code":-32601,"message":"Method not found"}}}`
	var msg MCPMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.NotNil(t, msg.Payload)
	assert.NotNil(t, msg.Payload.Error)
	assert.Equal(t, -32601, msg.Payload.Error.Code)
	assert.Equal(t, "Method not found", msg.Payload.Error.Message)
}

func TestIoTMessage_Unmarshal(t *testing.T) {
	data := `{"type":"iot","session_id":"sess-123","msgId":"iot-456","state":{"power":"on","brightness":80}}`
	var msg IoTMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, IoT, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.Equal(t, "iot-456", msg.MsgID)
	assert.NotNil(t, msg.State)
}

func TestAlertMessage_Unmarshal(t *testing.T) {
	data := `{"type":"alert","status":"Warning","message":"语音识别失败"}`
	var msg AlertMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, Alert, msg.Type)
	assert.Equal(t, "Warning", msg.Status)
	assert.Equal(t, "语音识别失败", msg.Message)
}

func TestAlertMessage_RoundTrip(t *testing.T) {
	original := AlertMessage{Type: Alert, Status: "Error", Message: "连接超时"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded AlertMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, original.Status, decoded.Status)
	assert.Equal(t, original.Message, decoded.Message)
}

func TestSystemMessage_Unmarshal(t *testing.T) {
	data := `{"type":"system","event":"session_start","session_id":"sess-xyz"}`
	var msg SystemMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, System, msg.Type)
	assert.Equal(t, "session_start", msg.Event)
	assert.Equal(t, "sess-xyz", msg.SessionID)
}

func TestSystemMessage_RoundTrip(t *testing.T) {
	original := SystemMessage{Type: System, Event: "session_end", SessionID: "sess-end"}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded SystemMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, original.Event, decoded.Event)
	assert.Equal(t, original.SessionID, decoded.SessionID)
}
