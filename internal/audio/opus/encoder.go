package opus

import (
	"errors"
)

type Encoder struct {
	channels int
}

func NewEncoder(sampleRate, channels, application int) (*Encoder, error) {
	return nil, errors.New("opus encoding not supported in pure-Go pion/opus; use Decoder for decode-only MVP; P5+ requires encoding then switch libopus via CGO route")
}

func (e *Encoder) Encode(pcm []int16, frameSize int) ([]byte, error) {
	return nil, errors.New("opus encoding not supported in pure-Go pion/opus; use Decoder for decode-only MVP; P5+ requires encoding then switch libopus via CGO route")
}
