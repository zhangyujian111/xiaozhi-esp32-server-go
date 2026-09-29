package protocol

type AlertMessage struct {
	Type    MessageType `json:"type"`
	Status  string      `json:"status,omitempty"`
	Message string      `json:"message,omitempty"`
}
