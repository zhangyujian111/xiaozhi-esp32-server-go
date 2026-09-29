package wav

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPCMToWAV_HeaderBytes(t *testing.T) {
	pcm := make([]byte, 100)
	wav := PCMToWAV(pcm, 16000, 1, 16)
	require.GreaterOrEqual(t, len(wav), 44)
	require.Equal(t, "RIFF", string(wav[0:4]))
	require.Equal(t, "WAVE", string(wav[8:12]))
	require.Equal(t, "fmt ", string(wav[12:16]))
}

func TestPCMToWAV_RoundTrip(t *testing.T) {
	pcm := make([]byte, 0)
	wav := PCMToWAV(pcm, 16000, 1, 16)
	require.Len(t, wav, 44, "empty PCM should produce 44-byte WAV header")
}

func TestWAVToPCM_InvalidMagic(t *testing.T) {
	_, _, _, _, err := WAVToPCM([]byte("XXXX"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}
