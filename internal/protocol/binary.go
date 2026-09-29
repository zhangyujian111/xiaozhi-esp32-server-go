package protocol

import "encoding/binary"

var LittleEndian = binary.LittleEndian

type BinaryV2Header struct {
	Version     uint16
	Type        uint16
	Reserved    uint32
	Timestamp   uint32
	PayloadSize uint32
}

type BinaryV3Header struct {
	Type        uint8
	Reserved    uint8
	PayloadSize uint16
}

func ParseBinaryFrame(data []byte, version int) (frameType int, timestamp uint32, payload []byte, err error) {
	switch version {
	case 1:
		return parseBinaryV1(data)
	case 2:
		return parseBinaryV2(data)
	case 3:
		return parseBinaryV3(data)
	default:
		return 0, 0, nil, ErrUnsupportedProtocolVersion
	}
}

func parseBinaryV1(data []byte) (int, uint32, []byte, error) {
	if len(data) == 0 {
		return 0, 0, nil, ErrShortBinaryFrame
	}
	return 0, 0, data, nil
}

func parseBinaryV2(data []byte) (int, uint32, []byte, error) {
	headerSize := 16
	if len(data) < headerSize {
		return 0, 0, nil, ErrShortBinaryFrame
	}
	payloadSize := LittleEndian.Uint32(data[12:16])
	totalSize := headerSize + int(payloadSize)
	if len(data) < totalSize {
		return 0, 0, nil, ErrInvalidPayloadSize
	}
	frameType := int(LittleEndian.Uint16(data[2:4]))
	timestamp := LittleEndian.Uint32(data[8:12])
	payload := data[headerSize:totalSize]
	return frameType, timestamp, payload, nil
}

func parseBinaryV3(data []byte) (int, uint32, []byte, error) {
	headerSize := 4
	if len(data) < headerSize {
		return 0, 0, nil, ErrShortBinaryFrame
	}
	payloadSize := LittleEndian.Uint16(data[2:4])
	if len(data) < headerSize+int(payloadSize) {
		return 0, 0, nil, ErrInvalidPayloadSize
	}
	frameType := int(data[0])
	return frameType, 0, data[headerSize : headerSize+int(payloadSize)], nil
}
