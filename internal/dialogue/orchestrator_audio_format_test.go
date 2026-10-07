package dialogue

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/rs/zerolog"
	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
)

func zeroLogger() zerolog.Logger { return zerolog.Nop() }

// TestPlayAudioToClient_WAV_DecodesAndEncodesOpusFrames 验证 aisaas 返回 wav 时
// 必须 wav.WAVToPCM -> opus.Encode -> 逐帧 WriteMessage(2, ...)。
// 历史 bug: playOpusToClient 直接 conn.WriteMessage(2, wavBytes)，把 WAV 当 raw opus 发给
// ESP32，ESP32 解码失败导致无声。
func TestPlayAudioToClient_WAV_DecodesAndEncodesOpusFrames(t *testing.T) {
	const sampleRate = 24000
	const channels = 1
	const bitsPerSample = 16
	const durationMS = 240 // 4 frames * 60ms
	pcmSamples := make([]int16, sampleRate*durationMS/1000)
	for i := range pcmSamples {
		pcmSamples[i] = int16(i % 1000)
	}
	pcmBytes := make([]byte, len(pcmSamples)*2)
	for i, s := range pcmSamples {
		binary.LittleEndian.PutUint16(pcmBytes[2*i:], uint16(s))
	}
	wavBytes := wav.PCMToWAV(pcmBytes, sampleRate, channels, bitsPerSample)

	var encodeCalls int
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			encodeCalls++
			return []byte{0xAA, 0xBB, 0xCC}, nil
		},
	}

	var msgCount int
	conn := &mockConn{
		WriteMessageFunc: func(msgType int, data []byte) error {
			if msgType == 2 {
				msgCount++
			}
			return nil
		},
	}

	o := &Orchestrator{opusEnc: enc, log: zeroLogger()}
	if err := o.playAudioToClient(context.Background(), "test-session", conn, wavBytes, sampleRate, "wav"); err != nil {
		t.Fatalf("playAudioToClient returned err: %v", err)
	}
	if encodeCalls != 4 {
		t.Fatalf("expected 4 opus encode calls (240ms / 60ms), got %d", encodeCalls)
	}
	if msgCount != 4 {
		t.Fatalf("expected 4 binary WriteMessage calls, got %d", msgCount)
	}
}

// TestPlayAudioToClient_Opus_PassesRawBytesThrough 验证 audioFormat=opus 时维持原行为 (raw forward).
func TestPlayAudioToClient_Opus_PassesRawBytesThrough(t *testing.T) {
	rawOpus := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			t.Fatal("opus encoder must NOT be called when audioFormat=opus")
			return nil, nil
		},
	}

	var msgCount int
	var sentBytes []byte
	conn := &mockConn{
		WriteMessageFunc: func(msgType int, data []byte) error {
			msgCount++
			sentBytes = data
			return nil
		},
	}

	o := &Orchestrator{opusEnc: enc, log: zeroLogger()}
	if err := o.playAudioToClient(context.Background(), "s", conn, rawOpus, 24000, "opus"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("expected 1 WriteMessage call (raw forward), got %d", msgCount)
	}
	if string(sentBytes) != string(rawOpus) {
		t.Fatalf("expected raw opus passthrough, got different bytes")
	}
}

// TestPlayAudioToClient_EmptyAudio_NoOp 验证 audio 为空时返回 nil 不调 encoder / conn.
func TestPlayAudioToClient_EmptyAudio_NoOp(t *testing.T) {
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			t.Fatal("opus encoder must NOT be called for empty audio")
			return nil, nil
		},
	}
	var msgCount int
	conn := &mockConn{
		WriteMessageFunc: func(msgType int, data []byte) error {
			msgCount++
			return nil
		},
	}
	o := &Orchestrator{opusEnc: enc, log: zeroLogger()}
	if err := o.playAudioToClient(context.Background(), "s", conn, nil, 24000, "wav"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if msgCount != 0 {
		t.Fatalf("expected 0 WriteMessage calls, got %d", msgCount)
	}
}
