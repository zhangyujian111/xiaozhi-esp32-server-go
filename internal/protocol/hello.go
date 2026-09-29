package protocol

import "encoding/json"

type HelloFeatures struct {
	MCP bool `json:"mcp,omitempty"`
	AEC bool `json:"aec,omitempty"`
	VAD bool `json:"vad,omitempty"`
}

type AudioParams struct {
	Format        string `json:"format,omitempty"`
	SampleRate    int    `json:"sample_rate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	FrameDuration int    `json:"frame_duration,omitempty"`
}

type HelloMessage struct {
	Type        MessageType    `json:"type"`
	Version     int            `json:"version,omitempty"`
	Features    *HelloFeatures `json:"features,omitempty"`
	Transport   string         `json:"transport,omitempty"`
	AudioParams *AudioParams   `json:"audio_params,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
}

func NewServerHello(sessionID string, audioParams AudioParams) *HelloMessage {
	return &HelloMessage{
		Type:        Hello,
		Transport:   "websocket",
		SessionID:   sessionID,
		AudioParams: &audioParams,
	}
}

func (m *HelloMessage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type        MessageType    `json:"type"`
		Version     int            `json:"version"`
		Features    *HelloFeatures `json:"features"`
		Transport   string         `json:"transport"`
		AudioParams *AudioParams   `json:"audio_params"`
		SessionID   string         `json:"session_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	m.Type = raw.Type
	m.Version = raw.Version
	m.Features = raw.Features
	m.Transport = raw.Transport
	m.AudioParams = raw.AudioParams
	m.SessionID = raw.SessionID
	return nil
}
