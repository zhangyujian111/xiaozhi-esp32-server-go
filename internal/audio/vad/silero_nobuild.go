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

// StartSession 满足 vad.VAD 接口。Stateless stub 返回 nil Session，
// 让 caller fallback 到 Process()（VAD 接口注释 L41 明确允许）。
// 修复 internal/app/app.go build 阻塞：预先 bug，silero_nobuild.go 缺此方法。
func (v *SileroVAD) StartSession() Session {
	return nil
}

func (v *SileroVAD) Destroy() {}