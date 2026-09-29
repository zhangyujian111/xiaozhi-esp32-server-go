package protocol

import (
	"encoding/json"
	"fmt"
)

type MessageType string

const (
	Hello  MessageType = "hello"
	Listen MessageType = "listen"
	Abort  MessageType = "abort"
	Ack    MessageType = "ack"
	TTS    MessageType = "tts"
	STT    MessageType = "stt"
	LLM    MessageType = "llm"
	MCP    MessageType = "mcp"
	IoT    MessageType = "iot"
	Alert  MessageType = "alert"
	System MessageType = "system"
)

func MustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("MustMarshal: %v", err))
	}
	return data
}
