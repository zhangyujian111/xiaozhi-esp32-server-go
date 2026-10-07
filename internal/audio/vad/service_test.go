package vad

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockVAD struct {
	scores []float32
	errs   []error
	idx    int
}

func (m *mockVAD) Process(pcm []int16) (float32, error) {
	s := m.scores[m.idx]
	e := m.errs[m.idx]
	m.idx++
	return s, e
}

func (m *mockVAD) StartSession() Session { return nil }

// Reset implements Session for mockVAD tests.
func (m *mockVAD) Reset() {}

func TestService_SilenceToSpeechStart(t *testing.T) {
	// Java semantics require 2 consecutive frames >= speechTh for SpeechStart.
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.1, 0.1}, errs: []error{nil, nil, nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	// First frame: above threshold but count=1 < guard → Silence.
	require.Equal(t, Silence, svc.Feed(nil, nil), "1st speech frame waits for the 2-frame guard")
	// Second frame: count=2 → SpeechStart.
	require.Equal(t, SpeechStart, svc.Feed(nil, nil), "should transition to speech when score exceeds threshold on 2nd consecutive frame")
}

func TestService_SpeechToEnd(t *testing.T) {
	// Need 2 consecutive speech frames first, then enough silence frames to
	// trigger SpeechEnd (silenceMs=90 with msPerFrame=32 → 2 frames).
	scores := []float32{0.9, 0.9}
	for i := 0; i < 11; i++ {
		scores = append(scores, 0.1)
	}
	mock := &mockVAD{scores: scores, errs: make([]error, len(scores))}
	svc := NewService(mock, 0.5, 0.3, 90)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil) // 1st speech (Silence)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil), "2nd consecutive speech frame → SpeechStart")
	for i := 0; i < 5; i++ {
		last := svc.Feed(nil, nil)
		if last == SpeechEnd {
			return
		}
	}
	t.Fatal("should have ended speech within 5 silence frames (silenceMs=90 / msPerFrame=32 = 2)")
}

func TestService_SpeechContinue(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.8, 0.7, 0.6, 0.5}, errs: make([]error, 6)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil))
	for i := 2; i < 5; i++ {
		status := svc.Feed(nil, nil)
		require.Equal(t, SpeechContinue, status)
	}
}

func TestService_VADError(t *testing.T) {
	testErr := errors.New("vad error")
	mock := &mockVAD{scores: []float32{0.1}, errs: []error{testErr}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	status := svc.Feed(nil, nil)
	require.Equal(t, Error, status, "should return Error when VAD returns error")
}

func TestService_SpeechAfterSilenceThreshold(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.4, 0.4, 0.4, 0.4}, errs: make([]error, 6)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil))
	for i := 2; i < 6; i++ {
		status := svc.Feed(nil, nil)
		require.Equal(t, SpeechContinue, status)
	}
}

func TestService_ZeroSilenceFrames(t *testing.T) {
	// silenceMs=50 with msPerFrame=32 → 1 frame (50/32 = 1). One frame of
	// silence after speech → SpeechEnd.
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.1, 0.1}, errs: []error{nil, nil, nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 50)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil))
	status := svc.Feed(nil, nil)
	require.Equal(t, SpeechEnd, status, "after 1 silence frame (silenceMs=50, msPerFrame=32) speech should end")
}

func TestService_SpeechMidThreshold(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.4, 0.4, 0.4}, errs: []error{nil, nil, nil, nil, nil}}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil))
	status := svc.Feed(nil, nil)
	require.Equal(t, SpeechContinue, status, "mid threshold during speech resets silence count")
}

func TestService_SpeechContinue_StaysAboveThreshold(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.9, 0.9, 0.9}, errs: make([]error, 5)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil))
	for i := 0; i < 3; i++ {
		status := svc.Feed(nil, nil)
		require.Equal(t, SpeechContinue, status)
	}
}

func TestService_SpeechContinue_ResetSilenceCount(t *testing.T) {
	mock := &mockVAD{scores: []float32{0.9, 0.9, 0.1, 0.4, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}, errs: make([]error, 11)}
	svc := NewService(mock, 0.5, 0.3, 100)
	require.NotNil(t, svc)

	_ = svc.Feed(nil, nil)
	require.Equal(t, SpeechStart, svc.Feed(nil, nil))
	status := svc.Feed(nil, nil)
	require.Equal(t, SpeechContinue, status, "0.4 (mid threshold) resets silence count")
}