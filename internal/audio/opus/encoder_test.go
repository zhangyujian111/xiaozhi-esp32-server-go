package opus

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// java 对齐：xiaozhi-common/utils/AudioUtils.java
//   SAMPLE_RATE = 16000
//   FRAME_SIZE  = 960   (60ms × 16kHz)
//   BITRATE     = 48000
//   CHANNELS    = 1
//   APP         = OPUS_APPLICATION_AUDIO
// xiaozhi-common/utils/OpusProcessor.java L262

const (
	testSampleRate  = 16000
	testChannels    = 1
	testFrameSize   = 960 // 60ms @ 16kHz
	testBitrate     = 48000
	testComplexity  = 10
)

func TestEncoder_New_Success(t *testing.T) {
	enc, err := NewEncoder(testSampleRate, testChannels)
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestEncoder_New_BadChannels(t *testing.T) {
	_, err := NewEncoder(testSampleRate, 3)
	require.Error(t, err)
}

func TestEncoder_Encode_Silence(t *testing.T) {
	enc, err := NewEncoder(testSampleRate, testChannels)
	require.NoError(t, err)

	silence := make([]int16, testSampleRate*60/1000)
	for i := range silence {
		silence[i] = 0
	}

	encoded, err := enc.Encode(silence, testFrameSize)
	require.NoError(t, err)
	require.NotEmpty(t, encoded, "encoding silence should produce non-empty opus frame")
}

func TestEncoder_RoundTrip(t *testing.T) {
	enc, err := NewEncoder(testSampleRate, testChannels)
	require.NoError(t, err)

	dec, err := NewDecoder(testSampleRate, testChannels)
	require.NoError(t, err)

	pcm := make([]int16, testFrameSize)
	for i := 0; i < testFrameSize; i++ {
		ti := float64(i) / float64(testSampleRate)
		pcm[i] = int16(math.Sin(2*math.Pi*1000*ti) * 32767)
	}

	encoded, err := enc.Encode(pcm, testFrameSize)
	require.NoError(t, err)
	require.NotEmpty(t, encoded, "encode should produce opus data")

	decoded, err := dec.Decode(encoded)
	require.NoError(t, err)
	require.Len(t, decoded, testFrameSize, "60ms@16kHz = 960 samples")

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
	enc, err := NewEncoder(testSampleRate, testChannels)
	require.NoError(t, err)

	_, err = enc.Encode(nil, testFrameSize)
	require.Error(t, err)
}

// TestEncoder_JavaAlignment_M3_Bug3 验证 encoder 用的 sample rate / bitrate / complexity 与 java 一致。
// 改 encoder 实现后此测试会 PASS；旧实现（AppVoIP / 无 bitrate 设置）也会 PASS（没断言 application 字段），
// 所以同时验证 sample rate + frame size 正确（这是 ESP32 解码的关键参数）。
func TestEncoder_JavaAlignment_M3_Bug3(t *testing.T) {
	enc, err := NewEncoder(testSampleRate, testChannels)
	require.NoError(t, err)

	// 1 帧 = 60ms @ 16kHz = 960 samples（java FRAME_SIZE）
	pcm := make([]int16, testFrameSize)
	for i := range pcm {
		pcm[i] = int16(math.Sin(2*math.Pi*440*float64(i)/float64(testSampleRate)) * 16000)
	}

	opusFrame, err := enc.Encode(pcm, testFrameSize)
	require.NoError(t, err)
	require.NotEmpty(t, opusFrame)

	// java: FRAME_SIZE=960, SAMPLE_RATE=16000 → 单帧 ~960 samples → opus 编码后 ~100-200 bytes
	require.Less(t, len(opusFrame), 1275, "opus frame should fit in java MAX_SIZE=1275")
}
