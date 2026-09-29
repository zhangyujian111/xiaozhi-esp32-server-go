package protocol

type STTMessage struct {
	Type    MessageType `json:"type"`
	Text    string      `json:"text,omitempty"`
	Emotion string      `json:"emotion,omitempty"`
}
