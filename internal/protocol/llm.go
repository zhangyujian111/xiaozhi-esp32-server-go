package protocol

type LLMMessage struct {
	Type    MessageType `json:"type"`
	Emotion string      `json:"emotion,omitempty"`
	Action  string      `json:"action,omitempty"`
	Text    string      `json:"text,omitempty"`
}
