package opus

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func generateSineWave(frequency float64, durationMs int, sampleRate int) []int16 {
	nSamples := sampleRate * durationMs / 1000
	buf := make([]int16, nSamples)
	for i := 0; i < nSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := math.Sin(2 * math.Pi * frequency * t)
		buf[i] = int16(val * 32767)
	}
	return buf
}

func TestDecoder_New_Success(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)
	require.NotNil(t, dec)
}

func TestDecoder_New_BadChannels(t *testing.T) {
	_, err := NewDecoder(16000, 3)
	require.Error(t, err)
}

func TestDecoder_Decode_ValidOpusFrame(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	pcm := generateSineWave(1000, 60, 16000)
	require.Len(t, pcm, 960, "60ms@16kHz = 960 samples")

	enc, err := NewEncoder(16000, 1)
	require.NoError(t, err)
	encoded, err := enc.Encode(pcm, 960)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := dec.Decode(encoded)
	require.NoError(t, err)
	require.Len(t, decoded, 960, "60ms@16kHz = 960 samples")

	var energy int64
	for _, s := range decoded {
		energy += int64(s) * int64(s)
	}
	require.Greater(t, energy, int64(100000), "decoded signal should have energy")
}

func TestDecoder_Decode_InvalidOpusData(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	_, err = dec.Decode([]byte{0xFF, 0xFE})
	require.Error(t, err)
}

func TestDecoder_Decode_NilInput(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	_, err = dec.Decode(nil)
	require.Error(t, err)
}

func TestDecoder_Decode_EmptyInput(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	_, err = dec.Decode([]byte{})
	require.Error(t, err)
}
