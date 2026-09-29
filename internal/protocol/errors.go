package protocol

import "errors"

var (
	ErrShortBinaryFrame           = errors.New("binary frame too short")
	ErrInvalidPayloadSize         = errors.New("invalid payload size")
	ErrUnsupportedProtocolVersion = errors.New("unsupported protocol version")
	ErrMissingDeviceID            = errors.New("missing device id")
	ErrInvalidToken               = errors.New("invalid token")
	ErrMalformedJSON              = errors.New("malformed JSON")
	ErrUnknownMessageType         = errors.New("unknown message type")
)

const (
	CloseNormalClosure           = 1000
	CloseGoingAway               = 1001
	CloseUnsupportedData         = 1003
	CloseInvalidFramePayloadData = 1007
	ClosePolicyViolation         = 1008
	CloseInternalServerErr       = 1011
)
