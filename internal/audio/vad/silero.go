//go:build silero

package vad

import "errors"

type SileroVAD struct{}

func NewSileroVAD(modelPath string) (*SileroVAD, error) {
	return nil, errors.New("silero VAD requires ONNX runtime")
}

func (v *SileroVAD) Process(pcm []int16) (float32, error) {
	return 0, errors.New("silero VAD requires ONNX runtime")
}
