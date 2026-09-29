package protocol

type SystemMessage struct {
	Type      MessageType `json:"type"`
	Event     string      `json:"event,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
}
