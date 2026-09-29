package protocol

import "encoding/json"

type MCPError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type MCPJSONRPC struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

type MCPMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Payload   *MCPJSONRPC `json:"payload,omitempty"`
}
