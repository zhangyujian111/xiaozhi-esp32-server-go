package protocol

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// Conn BinaryWriter 依赖的最小 interface（与 gorilla/websocket.Conn 兼容，便于 mock）。
type Conn interface {
	WriteMessage(msgType int, data []byte) error
	WriteJSON(v interface{}) error
}

// BinaryWriter 把 opus 帧按 ESP32 协议 v1/v2/v3 写到 ws conn。
//
// ESP32 期望 BigEndian header（hobbies htons/htonl），与 xz-go 现有的 LittleEndian
// BinaryV2Header parser 不同——WriteBinaryWriter 是发送方，header 必须 BE。
//
// v1: 裸 opus bytes（默认 / 协商失败）
// v2: 16-byte BE header + payload
// v3: 4-byte BE header + payload
type BinaryWriter struct {
	conn      Conn
	version   string
	timestamp uint32 // 采样数（每帧 960 samples @ 16kHz = 60ms）
	frameN    atomic.Uint32
}

// NewBinaryWriter 构造 writer，version 字符串（"1"/"2"/"3"）；空字符串默认 v1。
func NewBinaryWriter(conn Conn, version string) *BinaryWriter {
	if version == "" {
		version = "1"
	}
	return &BinaryWriter{conn: conn, version: version}
}

// WithTimestamp 返回带指定初始 timestamp 的新 writer（不修改原实例）。
func (w *BinaryWriter) WithTimestamp(ts uint32) *BinaryWriter {
	cp := *w
	cp.timestamp = ts
	cp.frameN.Store(0)
	return &cp
}

// Timestamp 返回当前 timestamp。
func (w *BinaryWriter) Timestamp() uint32 { return w.timestamp }

// WriteAudioFrame 按协商版本写入 1 帧 opus bytes。
//
// v2/v3 模式下，timestamp 自增（每帧 960 samples = 60ms @ 16kHz）。
// 调用 WithTimestamp(0) 可让 timestamp 从 0 起步；之后每帧 +960。
func (w *BinaryWriter) WriteAudioFrame(opusBytes []byte) error {
	switch w.version {
	case "1", "v1":
		return w.conn.WriteMessage(websocket.BinaryMessage, opusBytes)
	case "2", "v2":
		frame := make([]byte, 16+len(opusBytes))
		binary.BigEndian.PutUint16(frame[0:2], 2)             // version
		binary.BigEndian.PutUint16(frame[2:4], 0)             // type=audio
		binary.BigEndian.PutUint32(frame[4:8], 0)             // reserved
		binary.BigEndian.PutUint32(frame[8:12], w.timestamp)  // timestamp (samples)
		binary.BigEndian.PutUint32(frame[12:16], uint32(len(opusBytes)))
		copy(frame[16:], opusBytes)
		w.frameN.Add(1)
		w.timestamp += 960 // 60ms @ 16kHz = 960 samples
		return w.conn.WriteMessage(websocket.BinaryMessage, frame)
	case "3", "v3":
		frame := make([]byte, 4+len(opusBytes))
		frame[0] = 0 // type=audio
		frame[1] = 0 // reserved
		binary.BigEndian.PutUint16(frame[2:4], uint16(len(opusBytes)))
		copy(frame[4:], opusBytes)
		return w.conn.WriteMessage(websocket.BinaryMessage, frame)
	default:
		return fmt.Errorf("binarywriter: unsupported protocol version %q", w.version)
	}
}

// WriteText 写入 JSON text 控制消息（tts-start / sentence-start / tts-stop 等）。
func (w *BinaryWriter) WriteText(payload interface{}) error {
	return w.conn.WriteJSON(payload)
}