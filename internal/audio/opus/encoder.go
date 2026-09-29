package opus

import (
	"github.com/hraban/opus"
)

type Encoder struct {
	sampleRate int
	channels   int
	enc        *opus.Encoder
}

func NewEncoder(sampleRate, channels int) (*Encoder, error) {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}
	return &Encoder{sampleRate: sampleRate, channels: channels, enc: enc}, nil
}

func (e *Encoder) Encode(pcm []int16, frameSize int) ([]byte, error) {
	data := make([]byte, 4000)
	n, err := e.enc.Encode(pcm, data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}
