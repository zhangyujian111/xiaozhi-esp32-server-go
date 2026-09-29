package opus

import (
	"github.com/hraban/opus"
)

type Decoder struct {
	sampleRate int
	channels   int
	dec        *opus.Decoder
}

func NewDecoder(sampleRate, channels int) (*Decoder, error) {
	dec, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, err
	}
	return &Decoder{sampleRate: sampleRate, channels: channels, dec: dec}, nil
}

func (d *Decoder) Decode(input []byte) ([]int16, error) {
	maxFrameSamples := d.sampleRate * 60 / 1000
	pcm := make([]int16, maxFrameSamples*d.channels)
	n, err := d.dec.Decode(input, pcm)
	if err != nil {
		return nil, err
	}
	return pcm[:n], nil
}
