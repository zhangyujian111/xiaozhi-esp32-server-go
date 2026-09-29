package opus

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpusDecoder_NewDecoder(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)
	require.NotNil(t, dec)
}

func TestOpusDecoder_DecodeShortPacket(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	out, err := dec.Decode([]byte{0x00})
	require.NoError(t, err)
	require.NotNil(t, out)
}

func TestOpusDecoder_DecodeNil(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	_, err = dec.Decode(nil)
	require.Error(t, err)
}

func TestOpusDecoder_DecodeEmpty(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	_, err = dec.Decode([]byte{})
	require.Error(t, err)
}

func TestOpusDecoder_MultipleDecode(t *testing.T) {
	dec, err := NewDecoder(16000, 1)
	require.NoError(t, err)

	out1, err := dec.Decode(validOpusPacket)
	require.NoError(t, err)
	require.Len(t, out1, 320)

	out2, err := dec.Decode(validOpusPacket)
	require.NoError(t, err)
	require.Len(t, out2, 320)
}
