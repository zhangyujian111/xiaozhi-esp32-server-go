package protocol

type ListenState string

const (
	ListenStateStart  ListenState = "start"
	ListenStateStop   ListenState = "stop"
	ListenStateDetect ListenState = "detect"
)

type ListenMode string

const (
	ListenModeAuto     ListenMode = "auto"
	ListenModeManual   ListenMode = "manual"
	ListenModeRealtime ListenMode = "realtime"
)

type ListenMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	State     ListenState `json:"state,omitempty"`
	Mode      ListenMode  `json:"mode,omitempty"`
	Text      string      `json:"text,omitempty"`
}
