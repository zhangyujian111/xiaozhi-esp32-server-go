package opus

import (
	"github.com/pion/opus"
)

type Decoder struct {
	dec      opus.Decoder
	channels int
}

func NewDecoder(sampleRate, channels int) (*Decoder, error) {
	dec, err := opus.NewDecoderWithOutput(sampleRate, channels)
	if err != nil {
		return nil, err
	}
	return &Decoder{dec: dec, channels: channels}, nil
}

func (d *Decoder) Decode(input []byte) ([]int16, error) {
	out := make([]int16, 8192)
	n, err := d.dec.DecodeToInt16(input, out)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}
