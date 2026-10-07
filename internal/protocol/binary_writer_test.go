package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/gorilla/websocket"
)

// fakeConn 实现 BinaryWriter 所需 Conn interface（test 专用）。
type fakeConn struct {
	WriteMessageFunc func(msgType int, data []byte) error
	WriteJSONFunc    func(v interface{}) error
	gotMessages      [][]byte
	gotMessageTypes  []int
	gotJSONPayloads  []interface{}
}

func (f *fakeConn) WriteMessage(msgType int, data []byte) error {
	if f.WriteMessageFunc != nil {
		if err := f.WriteMessageFunc(msgType, data); err != nil {
			return err
		}
	}
	f.gotMessages = append(f.gotMessages, append([]byte{}, data...))
	f.gotMessageTypes = append(f.gotMessageTypes, msgType)
	return nil
}

func (f *fakeConn) WriteJSON(v interface{}) error {
	if f.WriteJSONFunc != nil {
		if err := f.WriteJSONFunc(v); err != nil {
			return err
		}
	}
	f.gotJSONPayloads = append(f.gotJSONPayloads, v)
	return nil
}

// TestBinaryWriter_V1RawBytes 验证 v1: 直接发送 opus bytes（0 header）。
func TestBinaryWriter_V1RawBytes(t *testing.T) {
	conn := &fakeConn{}
	w := NewBinaryWriter(conn, "1")
	opus := []byte{0xAA, 0xBB, 0xCC, 0xDD}

	if err := w.WriteAudioFrame(opus); err != nil {
		t.Fatalf("WriteAudioFrame: %v", err)
	}

	if got := len(conn.gotMessages); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
	if got := conn.gotMessages[0]; !bytes.Equal(got, opus) {
		t.Errorf("payload = %v, want raw %v", got, opus)
	}
	if got := conn.gotMessageTypes[0]; got != websocket.BinaryMessage {
		t.Errorf("msgType = %d, want BinaryMessage", got)
	}
}

// TestBinaryWriter_V2HeaderBE 验证 v2: 16-byte BigEndian header + payload。
//
// v2 Header (BigEndian):
//
//	version (uint16) | type (uint16) | reserved (uint32) | timestamp (uint32) | payload_size (uint32)
func TestBinaryWriter_V2HeaderBE(t *testing.T) {
	conn := &fakeConn{}
	w := NewBinaryWriter(conn, "2").WithTimestamp(960)
	opus := []byte{0xAA, 0xBB, 0xCC, 0xDD}

	if err := w.WriteAudioFrame(opus); err != nil {
		t.Fatalf("WriteAudioFrame: %v", err)
	}

	if got := len(conn.gotMessages); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
	got := conn.gotMessages[0]
	wantHeader := 16
	if len(got) != wantHeader+len(opus) {
		t.Fatalf("frame len = %d, want %d", len(got), wantHeader+len(opus))
	}

	hdr := got[:wantHeader]
	if v := binary.BigEndian.Uint16(hdr[0:2]); v != 2 {
		t.Errorf("version = %d, want 2", v)
	}
	if v := binary.BigEndian.Uint16(hdr[2:4]); v != 0 {
		t.Errorf("type = %d, want 0 (audio)", v)
	}
	if v := binary.BigEndian.Uint32(hdr[4:8]); v != 0 {
		t.Errorf("reserved = %d, want 0", v)
	}
	if v := binary.BigEndian.Uint32(hdr[8:12]); v != 960 {
		t.Errorf("timestamp = %d, want 960", v)
	}
	if v := binary.BigEndian.Uint32(hdr[12:16]); v != uint32(len(opus)) {
		t.Errorf("payload_size = %d, want %d", v, len(opus))
	}
	if !bytes.Equal(got[wantHeader:], opus) {
		t.Errorf("payload = %v, want %v", got[wantHeader:], opus)
	}
}

// TestBinaryWriter_V3HeaderBE 验证 v3: 4-byte BigEndian header + payload。
//
// v3 Header (BigEndian):
//
//	type (uint8) | reserved (uint8) | payload_size (uint16)
func TestBinaryWriter_V3HeaderBE(t *testing.T) {
	conn := &fakeConn{}
	w := NewBinaryWriter(conn, "3")
	opus := []byte{0xAA, 0xBB, 0xCC, 0xDD}

	if err := w.WriteAudioFrame(opus); err != nil {
		t.Fatalf("WriteAudioFrame: %v", err)
	}

	got := conn.gotMessages[0]
	wantHeader := 4
	if len(got) != wantHeader+len(opus) {
		t.Fatalf("frame len = %d, want %d", len(got), wantHeader+len(opus))
	}

	hdr := got[:wantHeader]
	if v := hdr[0]; v != 0 {
		t.Errorf("type = %d, want 0", v)
	}
	if v := hdr[1]; v != 0 {
		t.Errorf("reserved = %d, want 0", v)
	}
	if v := binary.BigEndian.Uint16(hdr[2:4]); v != uint16(len(opus)) {
		t.Errorf("payload_size = %d, want %d", v, len(opus))
	}
}

// TestBinaryWriter_DefaultsToV1 验证空版本默认按 v1（裸字节）处理。
func TestBinaryWriter_DefaultsToV1(t *testing.T) {
	conn := &fakeConn{}
	w := NewBinaryWriter(conn, "")
	opus := []byte{0xAA, 0xBB}

	if err := w.WriteAudioFrame(opus); err != nil {
		t.Fatalf("WriteAudioFrame: %v", err)
	}
	if got := conn.gotMessages[0]; !bytes.Equal(got, opus) {
		t.Errorf("payload = %v, want raw %v", got, opus)
	}
}

// TestBinaryWriter_WriteTextDelegatesToConn 验证 WriteText 直接走 WriteJSON。
func TestBinaryWriter_WriteTextDelegatesToConn(t *testing.T) {
	type payload struct {
		Type string `json:"type"`
	}
	conn := &fakeConn{
		WriteJSONFunc: func(v interface{}) error { return nil },
	}
	w := NewBinaryWriter(conn, "1")

	if err := w.WriteText(payload{Type: "hello"}); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if got := len(conn.gotJSONPayloads); got != 1 {
		t.Errorf("WriteJSON call count = %d, want 1", got)
	}
}

// TestBinaryWriter_TimestampIncrement 验证连续写入时 timestamp 自增。
func TestBinaryWriter_TimestampIncrement(t *testing.T) {
	conn := &fakeConn{}
	w := NewBinaryWriter(conn, "2").WithTimestamp(0)
	opus := []byte{0x01}

	if err := w.WriteAudioFrame(opus); err != nil {
		t.Fatalf("WriteAudioFrame 1: %v", err)
	}
	if err := w.WriteAudioFrame(opus); err != nil {
		t.Fatalf("WriteAudioFrame 2: %v", err)
	}

	if len(conn.gotMessages) != 2 {
		t.Fatalf("message count = %d, want 2", len(conn.gotMessages))
	}
	ts1 := binary.BigEndian.Uint32(conn.gotMessages[0][8:12])
	ts2 := binary.BigEndian.Uint32(conn.gotMessages[1][8:12])
	if ts2 != ts1+960 {
		t.Errorf("ts2 = %d, want ts1+%d=%d", ts2, 960, ts1+960)
	}
}

// TestBinaryWriter_TimestampExplicit 验证 WithTimestamp 返回新实例（不修改原 writer）。
func TestBinaryWriter_TimestampExplicit(t *testing.T) {
	conn := &fakeConn{}
	base := NewBinaryWriter(conn, "2")
	w := base.WithTimestamp(480)

	if w.Timestamp() != 480 {
		t.Errorf("derived ts = %d, want 480", w.Timestamp())
	}
	if base.Timestamp() != 0 {
		t.Errorf("base ts mutated = %d, want 0", base.Timestamp())
	}
}