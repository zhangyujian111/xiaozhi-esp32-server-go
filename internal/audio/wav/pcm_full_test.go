package wav

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWAVToPCM_ValidWAV(t *testing.T) {
	pcm := make([]byte, 100)
	wav := PCMToWAV(pcm, 16000, 1, 16)
	out, sr, ch, bps, err := WAVToPCM(wav)
	require.NoError(t, err)
	require.Equal(t, 100, len(out))
	require.Equal(t, 16000, sr)
	require.Equal(t, 1, ch)
	require.Equal(t, 16, bps)
}

func TestWAVToPCM_ShortBuffer(t *testing.T) {
	_, _, _, _, err := WAVToPCM([]byte("RIFF"))
	require.Error(t, err)
}

func TestWAVToPCM_WrongFormat(t *testing.T) {
	wav := make([]byte, 44)
	wav[0] = 'X'
	wav[8] = 'W'
	_, _, _, _, err := WAVToPCM(wav)
	require.Error(t, err)
}

func TestWAVToPCM_WrongWave(t *testing.T) {
	wav := make([]byte, 44)
	wav[0] = 'R'
	wav[1] = 'I'
	wav[2] = 'F'
	wav[3] = 'F'
	wav[8] = 'W'
	wav[9] = 'A'
	wav[10] = 'V'
	wav[11] = 'X'
	_, _, _, _, err := WAVToPCM(wav)
	require.Error(t, err)
}

func TestWAVToPCM_WrongFmt(t *testing.T) {
	wav := make([]byte, 44)
	wav[0] = 'R'
	wav[1] = 'I'
	wav[2] = 'F'
	wav[3] = 'F'
	wav[8] = 'W'
	wav[9] = 'A'
	wav[10] = 'V'
	wav[11] = 'E'
	wav[12] = 'f'
	wav[13] = 'm'
	wav[14] = 't'
	wav[15] = 'X'
	_, _, _, _, err := WAVToPCM(wav)
	require.Error(t, err)
}

func TestWAVToPCM_FmtChunkSizeNot16(t *testing.T) {
	wav := make([]byte, 44)
	wav[0] = 'R'
	wav[1] = 'I'
	wav[2] = 'F'
	wav[3] = 'F'
	wav[8] = 'W'
	wav[9] = 'A'
	wav[10] = 'V'
	wav[11] = 'E'
	wav[12] = 'f'
	wav[13] = 'm'
	wav[14] = 't'
	wav[15] = ' '
	binary.LittleEndian.PutUint32(wav[16:20], 18)
	_, _, _, _, err := WAVToPCM(wav)
	require.Error(t, err)
}

func TestWAVToPCM_FormatNotPCM(t *testing.T) {
	wav := make([]byte, 44)
	wav[0] = 'R'
	wav[1] = 'I'
	wav[2] = 'F'
	wav[3] = 'F'
	wav[8] = 'W'
	wav[9] = 'A'
	wav[10] = 'V'
	wav[11] = 'E'
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 3)
	_, _, _, _, err := WAVToPCM(wav)
	require.Error(t, err)
}

func TestPCMToWAV_Stereo(t *testing.T) {
	pcm := make([]byte, 100)
	wav := PCMToWAV(pcm, 44100, 2, 16)
	require.GreaterOrEqual(t, len(wav), 44)
	require.Equal(t, "RIFF", string(wav[0:4]))
	require.Equal(t, "WAVE", string(wav[8:12]))
}

func TestPCMToWAV_48kHz(t *testing.T) {
	pcm := make([]byte, 100)
	wav := PCMToWAV(pcm, 48000, 1, 16)
	require.GreaterOrEqual(t, len(wav), 44)
}

func TestPCMToWAV_EmptyPCM(t *testing.T) {
	pcm := make([]byte, 0)
	wav := PCMToWAV(pcm, 16000, 1, 16)
	require.Len(t, wav, 44)
}
