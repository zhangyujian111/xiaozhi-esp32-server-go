package protocol

type IoTMessage struct {
	Type      MessageType    `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	MsgID     string         `json:"msgId,omitempty"`
	State     map[string]any `json:"state,omitempty"`
}
