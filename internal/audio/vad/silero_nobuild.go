//go:build !silero

package vad

import "errors"

// SileroVAD is a stub when built without the `silero` tag. The ONNX runtime
// is not pulled in, so model loading is unavailable. Build with
// `-tags silero` to enable real Silero VAD.
type SileroVAD struct{}

func NewSileroVAD(modelPath string, sharedLibPath ...string) (*SileroVAD, error) {
	return nil, errors.New("silero VAD not built: use `-tags silero` to enable")
}

func (v *SileroVAD) Process(pcm []int16) (float32, error) {
	return 0, errors.New("silero VAD not built: use `-tags silero` to enable")
}

func (v *SileroVAD) Destroy() {}