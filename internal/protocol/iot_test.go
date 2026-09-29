package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIoTMessage_RoundTrip(t *testing.T) {
	original := IoTMessage{Type: IoT, SessionID: "sess-iot", MsgID: "msg-iot", State: map[string]any{"power": "off"}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded IoTMessage
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	assert.Equal(t, original.SessionID, decoded.SessionID)
	assert.Equal(t, original.MsgID, decoded.MsgID)
}

func TestIoTMessage_NoMsgID(t *testing.T) {
	data := `{"type":"iot","session_id":"sess-123","state":{"temp":25}}`
	var msg IoTMessage
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	assert.Equal(t, IoT, msg.Type)
	assert.Equal(t, "sess-123", msg.SessionID)
	assert.Empty(t, msg.MsgID)
}
