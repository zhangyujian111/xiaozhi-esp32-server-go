package protocol

type AckMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"sessionId,omitempty"`
	MsgType   string      `json:"msgType,omitempty"`
	MsgID     string      `json:"msgId,omitempty"`
	Status    string      `json:"status,omitempty"`
	Error     string      `json:"error,omitempty"`
}
