//go:build !silero

package vad

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSileroNoBuild_ImplementsVADInterface 验证 silero_nobuild stub 满足 vad.VAD 接口。
// VAD 注释明确允许 stateless 实现返回 nil Session（fallback 到 Process）。
// 预先 bug：silero_nobuild.go 缺 StartSession() 方法导致 internal/app/app.go build 失败。
func TestSileroNoBuild_ImplementsVADInterface(t *testing.T) {
	v, err := NewSileroVAD("model.onnx")
	require.Error(t, err)
	require.Nil(t, v)

	// 直接测 stub：构造空 SileroVAD，验证 StartSession 返回 nil（stateless fallback）
	stub := &SileroVAD{}
	sess := stub.StartSession()
	require.Nil(t, sess, "stateless stub should return nil Session (fallback to Process)")

	// Process stub 应该返回错误（无 ONNX runtime）
	prob, perr := stub.Process(nil)
	require.Error(t, perr)
	require.Equal(t, float32(0), prob)

	// Destroy 不能 panic
	require.NotPanics(t, func() { stub.Destroy() })
}
