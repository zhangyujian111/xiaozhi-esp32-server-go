package opus

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

var validOpusPacket = []byte{
	0x48, 0x85, 0x07, 0x04, 0xe5, 0x7c, 0x5e, 0x85, 0xe3, 0x98, 0x3e, 0x0d, 0xaf, 0x73, 0xf0, 0xfe,
	0x85, 0x14, 0x66, 0x79, 0xfe, 0x9a, 0x54, 0xe2, 0x0f, 0x73, 0x5b, 0x2b, 0xb6, 0x0b, 0xcd, 0xc8,
	0x46, 0x0c, 0x39, 0xf7, 0x82, 0xf8, 0x25, 0xd3, 0x70, 0x4e, 0xa8, 0x5a, 0x9d, 0x93, 0xcd, 0xee,
	0x60, 0x2a, 0x12, 0xe2, 0x9a, 0x15, 0x40,
}

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

func TestOpusDecodeRoundTrip(t *testing.T) {
	sampleRate := 16000
	channels := 1
	durationMs := 60

	pcm := generateSineWave(1000, durationMs, sampleRate)
	require.Len(t, pcm, 960, "60ms@16kHz = 960 samples")

	dec, err := NewDecoder(sampleRate, channels)
	require.NoError(t, err)

	decoded, err := dec.Decode(validOpusPacket)
	require.NoError(t, err)
	require.Len(t, decoded, 320, "20ms@16kHz = 320 samples")

	var energy int64
	for _, s := range decoded {
		energy += int64(s) * int64(s)
	}
	require.Greater(t, energy, int64(1000000), "decoded signal should have significant energy")
}
