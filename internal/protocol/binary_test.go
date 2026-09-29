package protocol

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBinaryFrame_V1(t *testing.T) {
	payload := []byte("hello world")
	frameType, ts, payloadOut, err := ParseBinaryFrame(payload, 1)
	assert.NoError(t, err)
	assert.Equal(t, 0, frameType)
	assert.Equal(t, uint32(0), ts)
	assert.Equal(t, payload, payloadOut)
}

func TestParseBinaryFrame_V2(t *testing.T) {
	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint16(header[2:4], 0)
	binary.LittleEndian.PutUint32(header[4:8], 0)
	binary.LittleEndian.PutUint32(header[8:12], 12345)
	binary.LittleEndian.PutUint32(header[12:16], 11)
	payload := []byte("hello world")
	data := append(header, payload...)

	frameType, ts, payloadOut, err := ParseBinaryFrame(data, 2)
	assert.NoError(t, err)
	assert.Equal(t, 0, frameType)
	assert.Equal(t, uint32(12345), ts)
	assert.Equal(t, []byte("hello world"), payloadOut)
}

func TestParseBinaryFrame_V2_JSON(t *testing.T) {
	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint16(header[2:4], 1)
	binary.LittleEndian.PutUint32(header[4:8], 0)
	binary.LittleEndian.PutUint32(header[8:12], 0)
	payload := []byte(`{"a":1}`)
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(payload)))
	data := append(header, payload...)

	frameType, _, payloadOut, err := ParseBinaryFrame(data, 2)
	assert.NoError(t, err)
	assert.Equal(t, 1, frameType)
	assert.Equal(t, []byte(`{"a":1}`), payloadOut)
}

func TestParseBinaryFrame_V3(t *testing.T) {
	header := make([]byte, 4)
	header[0] = 0
	header[1] = 0
	binary.LittleEndian.PutUint16(header[2:4], 11)
	payload := []byte("hello world")
	data := append(header, payload...)

	frameType, _, payloadOut, err := ParseBinaryFrame(data, 3)
	assert.NoError(t, err)
	assert.Equal(t, 0, frameType)
	assert.Equal(t, []byte("hello world"), payloadOut)
}

func TestParseBinaryFrame_V3_JSON(t *testing.T) {
	header := make([]byte, 4)
	header[0] = 1
	header[1] = 0
	payload := []byte(`{"b":2}`)
	binary.LittleEndian.PutUint16(header[2:4], uint16(len(payload)))
	data := append(header, payload...)

	frameType, _, payloadOut, err := ParseBinaryFrame(data, 3)
	assert.NoError(t, err)
	assert.Equal(t, 1, frameType)
	assert.Equal(t, []byte(`{"b":2}`), payloadOut)
}

func TestParseBinaryFrame_ShortFrame_V2(t *testing.T) {
	data := make([]byte, 15)
	_, _, _, err := ParseBinaryFrame(data, 2)
	assert.ErrorIs(t, err, ErrShortBinaryFrame)
}

func TestParseBinaryFrame_ShortFrame_V3(t *testing.T) {
	data := []byte{0, 0, 0}
	_, _, _, err := ParseBinaryFrame(data, 3)
	assert.ErrorIs(t, err, ErrShortBinaryFrame)
}

func TestParseBinaryFrame_UnsupportedVersion(t *testing.T) {
	_, _, _, err := ParseBinaryFrame([]byte("test"), 99)
	assert.ErrorIs(t, err, ErrUnsupportedProtocolVersion)
}

func TestParseBinaryFrame_PayloadSizeMismatch(t *testing.T) {
	header := make([]byte, 16)
	binary.LittleEndian.PutUint16(header[0:2], 2)
	binary.LittleEndian.PutUint16(header[2:4], 0)
	binary.LittleEndian.PutUint32(header[4:8], 0)
	binary.LittleEndian.PutUint32(header[8:12], 0)
	binary.LittleEndian.PutUint32(header[12:16], 100)
	payload := []byte("short")
	data := append(header, payload...)

	_, _, _, err := ParseBinaryFrame(data, 2)
	assert.ErrorIs(t, err, ErrInvalidPayloadSize)
}
