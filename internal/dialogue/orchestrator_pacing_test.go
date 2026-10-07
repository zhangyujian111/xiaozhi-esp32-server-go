package dialogue

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/xiaozhi/xiaozhi-esp32-server-go/internal/audio/wav"
)

// mockFramePacer records every Pace() invocation (frame index + timestamp).
// Used to assert pacing order without actually sleeping.
type mockFramePacer struct {
	calls []mockPaceCall
}

type mockPaceCall struct {
	frameIdx int
	at       time.Time
}

func (p *mockFramePacer) Pace(_ context.Context, frameIdx int) {
	p.calls = append(p.calls, mockPaceCall{frameIdx: frameIdx, at: time.Now()})
}

// TestPlayWAVAsOpusFrames_PacingEnabled_PaceCalledPerFrame 验证：当注入 FramePacer 时，
// playWAVAsOpusFrames 在每帧发送前调一次 Pace(frameIdx)。
//
// 背景（Bug3 修复）：xz-go 此前把 tts start JSON 和所有 opus frames 一次推入 ESP32 websocket
// buffer。ESP32 处理 tts start JSON 异步触发 SetDeviceState(Speaking)（Schedule lambda 推
// main thread queue），但紧接着就处理 opus frame 1，device_state 还是 listening → drop。
//
// Java ScheduledPlayer.sendSpeechWithBurstMode 解决方案：前 2 帧立即发（prebuffer），后续
// 每 60ms 一帧，给 ESP32 main thread 留时间切 state。xz-go 同样需要 pacing。
func TestPlayWAVAsOpusFrames_PacingEnabled_PaceCalledPerFrame(t *testing.T) {
	const sampleRate = 16000
	const durationMS = 240 // 4 frames * 60ms
	pcmSamples := make([]int16, sampleRate*durationMS/1000)
	for i := range pcmSamples {
		pcmSamples[i] = int16(i % 1000)
	}
	pcmBytes := make([]byte, len(pcmSamples)*2)
	for i, s := range pcmSamples {
		binary.LittleEndian.PutUint16(pcmBytes[2*i:], uint16(s))
	}
	wavBytes := wav.PCMToWAV(pcmBytes, sampleRate, 1, 16)

	pacer := &mockFramePacer{}
	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte{0xAA}, nil
		},
	}
	conn := &mockConn{}

	o := &Orchestrator{opusEnc: enc, framePacer: pacer, log: zeroLogger()}
	if err := o.playWAVAsOpusFrames(context.Background(), "s", conn, wavBytes, sampleRate); err != nil {
		t.Fatalf("playWAVAsOpusFrames: %v", err)
	}

	// 4 帧 → 4 次 Pace 调用，frameIdx 严格递增 0..3
	if len(pacer.calls) != 4 {
		t.Fatalf("expected 4 Pace calls (one per frame), got %d", len(pacer.calls))
	}
	for i, c := range pacer.calls {
		if c.frameIdx != i {
			t.Fatalf("Pace call %d frameIdx=%d, expected %d", i, c.frameIdx, i)
		}
	}
}

// TestPlayWAVAsOpusFrames_NoPacer_NoError 验证 nil pacer 不报错（向后兼容）。
func TestPlayWAVAsOpusFrames_NoPacer_NoError(t *testing.T) {
	const sampleRate = 16000
	const durationMS = 120 // 2 frames
	pcmSamples := make([]int16, sampleRate*durationMS/1000)
	pcmBytes := make([]byte, len(pcmSamples)*2)
	for i, s := range pcmSamples {
		binary.LittleEndian.PutUint16(pcmBytes[2*i:], uint16(s))
	}
	wavBytes := wav.PCMToWAV(pcmBytes, sampleRate, 1, 16)

	enc := &mockOpusEncoder{
		EncodeFunc: func(pcm []int16, frameSize int) ([]byte, error) {
			return []byte{0xAA}, nil
		},
	}
	conn := &mockConn{}

	o := &Orchestrator{opusEnc: enc, framePacer: nil, log: zeroLogger()}
	if err := o.playWAVAsOpusFrames(context.Background(), "s", conn, wavBytes, sampleRate); err != nil {
		t.Fatalf("playWAVAsOpusFrames with nil pacer must not error: %v", err)
	}
}

// TestIntervalFramePacer_FirstFrameImmediate_ThenInterval 验证 intervalFramePacer
// 行为：frame 0 立即通过（prebuffer），frame 1+ 等 interval。
//
// Java ScheduledPlayer.sendSpeechWithBurstMode：playPosition 初始 -120ms，前 2 帧立即
// 发（prebuffer），后续 60ms 一帧。xz-go 用更简化的策略：第 1 帧立即发（prebuffer），后续
// 帧按 interval 节奏。
func TestIntervalFramePacer_FirstFrameImmediate_ThenInterval(t *testing.T) {
	const interval = 60 * time.Millisecond
	pacer := NewFramePacer(interval)

	// frame 0: must return immediately (no sleep)
	start := time.Now()
	pacer.Pace(context.Background(), 0)
	if elapsed := time.Since(start); elapsed > 5*time.Millisecond {
		t.Fatalf("frame 0 must return immediately, took %v", elapsed)
	}

	// frame 1: must wait ~interval
	start = time.Now()
	pacer.Pace(context.Background(), 1)
	elapsed := time.Since(start)
	if elapsed < interval-10*time.Millisecond {
		t.Fatalf("frame 1 must wait ~%v, took %v", interval, elapsed)
	}
	if elapsed > interval+30*time.Millisecond {
		t.Fatalf("frame 1 wait too long: %v (expected ~%v)", elapsed, interval)
	}

	// frame 2: must wait ~interval again
	start = time.Now()
	pacer.Pace(context.Background(), 2)
	elapsed = time.Since(start)
	if elapsed < interval-10*time.Millisecond {
		t.Fatalf("frame 2 must wait ~%v, took %v", interval, elapsed)
	}
}

// TestIntervalFramePacer_ContextCanceled 验证 ctx cancel 立即返回（不发帧不阻塞）。
func TestIntervalFramePacer_ContextCanceled(t *testing.T) {
	pacer := NewFramePacer(500 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	start := time.Now()
	pacer.Pace(ctx, 1) // frame 1 would normally wait 500ms
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("Pace must return immediately on canceled ctx, took %v", elapsed)
	}
}
