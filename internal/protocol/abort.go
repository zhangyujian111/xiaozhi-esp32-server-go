package protocol

type AbortMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Reason    string      `json:"reason,omitempty"`
}
