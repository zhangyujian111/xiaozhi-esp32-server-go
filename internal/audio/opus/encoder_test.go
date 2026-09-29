package opus

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncoder_New_Success(t *testing.T) {
	enc, err := NewEncoder(24000, 1)
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestEncoder_New_BadChannels(t *testing.T) {
	_, err := NewEncoder(24000, 3)
	require.Error(t, err)
}

func TestEncoder_Encode_Silence(t *testing.T) {
	enc, err := NewEncoder(24000, 1)
	require.NoError(t, err)

	silence := make([]int16, 24000*60/1000)
	for i := range silence {
		silence[i] = 0
	}

	encoded, err := enc.Encode(silence, 1440)
	require.NoError(t, err)
	require.NotEmpty(t, encoded, "encoding silence should produce non-empty opus frame")
}

func TestEncoder_RoundTrip(t *testing.T) {
	enc, err := NewEncoder(24000, 1)
	require.NoError(t, err)

	dec, err := NewDecoder(24000, 1)
	require.NoError(t, err)

	pcm := make([]int16, 1440)
	for i := 0; i < 1440; i++ {
		t := float64(i) / 24000.0
		pcm[i] = int16(math.Sin(2*math.Pi*1000*t) * 32767)
	}

	encoded, err := enc.Encode(pcm, 1440)
	require.NoError(t, err)
	require.NotEmpty(t, encoded, "encode should produce opus data")

	decoded, err := dec.Decode(encoded)
	require.NoError(t, err)
	require.Len(t, decoded, 1440, "60ms@24kHz = 1440 samples")

	var origEnergy, decodedEnergy int64
	for i := range decoded {
		origEnergy += int64(pcm[i]) * int64(pcm[i])
		decodedEnergy += int64(decoded[i]) * int64(decoded[i])
	}
	ratio := float64(decodedEnergy) / float64(origEnergy)
	require.Greater(t, ratio, 0.01, "decoded signal should retain some energy (ratio=%f)", ratio)
	require.Less(t, ratio, 2.0, "decoded energy should not be wildly different (ratio=%f)", ratio)
}

func TestEncoder_Encode_NilInput(t *testing.T) {
	enc, err := NewEncoder(24000, 1)
	require.NoError(t, err)

	_, err = enc.Encode(nil, 1440)
	require.Error(t, err)
}
