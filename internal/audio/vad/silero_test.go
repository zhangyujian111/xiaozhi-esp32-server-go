//go:build silero_and_test

package vad

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSileroVAD_RequiresOnnxRuntime(t *testing.T) {
	v, err := NewSileroVAD("model.onnx")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ONNX")
	require.Nil(t, v)
}
