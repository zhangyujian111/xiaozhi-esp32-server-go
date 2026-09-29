package protocol

type TTSState string

const (
	TTSStateStart         TTSState = "start"
	TTSStateSentenceStart TTSState = "sentence_start"
	TTSStateStop          TTSState = "stop"
)

type TTSMessage struct {
	Type  MessageType `json:"type"`
	State TTSState    `json:"state,omitempty"`
	Text  string      `json:"text,omitempty"`
}
