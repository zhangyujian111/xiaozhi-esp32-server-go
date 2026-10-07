package opus

import (
	"github.com/hraban/opus"
)

// java 对齐：xiaozhi-common/utils/AudioUtils.java: BITRATE=48000 (48kbps)
// java 对齐：xiaozhi-common/utils/OpusProcessor.java: OPUS_APPLICATION_AUDIO（高保真，TTS 更接近有声内容）
const (
	DefaultBitrate    = 48000
	DefaultComplexity = 10
)

type Encoder struct {
	sampleRate int
	channels   int
	enc        *opus.Encoder
}

func NewEncoder(sampleRate, channels int) (*Encoder, error) {
	enc, err := opus.NewEncoder(sampleRate, channels, opus.AppAudio)
	if err != nil {
		return nil, err
	}
	if err := enc.SetBitrate(DefaultBitrate); err != nil {
		return nil, err
	}
	if err := enc.SetComplexity(DefaultComplexity); err != nil {
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
